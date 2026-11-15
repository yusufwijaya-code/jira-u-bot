package chat_app_auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "chat"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func enc(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func sign(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	signing := enc(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// certServer serves the certificate document; fail answers 500, and a
// non-nil block holds every request until it is closed (started is
// signalled first).
type certServer struct {
	mu      sync.Mutex
	certs   map[string]string
	hits    int
	fail    bool
	block   chan struct{}
	started chan struct{}
}

func (s *certServer) start(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits++
		block, started, fail, certs := s.block, s.started, s.fail, s.certs
		s.mu.Unlock()
		if started != nil {
			started <- struct{}{}
		}
		if block != nil {
			<-block
		}
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(certs)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (s *certServer) hitCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Now()} }

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const aud = "275006361614"

func claimsAt(now time.Time) map[string]any {
	return map[string]any{"iss": ChatIssuer, "aud": aud, "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()}
}

func validClaims() map[string]any { return claimsAt(time.Now()) }

func TestVerify(t *testing.T) {
	key, cert := newKey(t)
	other, _ := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), time.Now)
	ctx := context.Background()

	if err := v.Verify(ctx, sign(t, key, "k1", validClaims())); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

	hsSigning := enc(map[string]string{"alg": "HS256", "kid": "k1"}) + "." + enc(validClaims())
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(hsSigning))

	cases := map[string]string{
		"wrong audience": sign(t, key, "k1", func() map[string]any { c := validClaims(); c["aud"] = "123"; return c }()),
		"wrong issuer":   sign(t, key, "k1", func() map[string]any { c := validClaims(); c["iss"] = "evil@example.com"; return c }()),
		"expired": sign(t, key, "k1", func() map[string]any {
			c := validClaims()
			c["exp"] = time.Now().Add(-2 * time.Minute).Unix()
			return c
		}()),
		"bad signature": sign(t, other, "k1", validClaims()),
		"unknown kid":   sign(t, key, "nope", validClaims()),
		"alg none":      enc(map[string]string{"alg": "none", "kid": "k1"}) + "." + enc(validClaims()) + ".",
		"alg HS256":     hsSigning + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
		"malformed":     "abc.def",
		"empty":         "",
	}
	for name, token := range cases {
		if err := v.Verify(ctx, token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v, want ErrInvalidToken", name, err)
		}
	}
}

func TestVerify_OversizedTokenIsRejectedBeforeAnyWork(t *testing.T) {
	cs := &certServer{certs: map[string]string{}}
	v := newVerifier(aud, cs.start(t), time.Now)
	token := enc(map[string]string{"alg": "RS256", "kid": "k1"}) + "." + strings.Repeat("A", MaxTokenBytes) + ".sig"
	err := v.Verify(context.Background(), token)
	if !errors.Is(err, ErrInvalidToken) || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want ErrInvalidToken (too large)", err)
	}
	if cs.hitCount() != 0 {
		t.Errorf("certificate fetches = %d, want 0", cs.hitCount())
	}
}

func TestVerify_CacheHitDoesNotRefetch(t *testing.T) {
	key, cert := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), time.Now)
	for i := 0; i < 3; i++ {
		if err := v.Verify(context.Background(), sign(t, key, "k1", validClaims())); err != nil {
			t.Fatal(err)
		}
	}
	if cs.hitCount() != 1 {
		t.Errorf("certificate fetches = %d, want 1", cs.hitCount())
	}
}

func TestVerify_RefetchesCertsForANewKid(t *testing.T) {
	key1, cert1 := newKey(t)
	key2, cert2 := newKey(t)
	clk := newClock()
	cs := &certServer{certs: map[string]string{"k1": cert1}}
	v := newVerifier(aud, cs.start(t), clk.now)
	ctx := context.Background()

	if err := v.Verify(ctx, sign(t, key1, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	cs.certs = map[string]string{"k2": cert2}
	cs.mu.Unlock()
	clk.advance(minRefreshInterval + time.Second)
	if err := v.Verify(ctx, sign(t, key2, "k2", claimsAt(clk.now()))); err != nil {
		t.Fatalf("rotated key rejected: %v", err)
	}
	if err := v.Verify(ctx, sign(t, key1, "k1", claimsAt(clk.now()))); err != nil && !strings.Contains(err.Error(), "kid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_UnknownKidRefetchIsRateLimited(t *testing.T) {
	key, cert := newKey(t)
	clk := newClock()
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), clk.now)
	ctx := context.Background()

	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := v.Verify(ctx, sign(t, key, "attacker", claimsAt(clk.now()))); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("unknown kid: err = %v", err)
		}
	}
	if cs.hitCount() != 1 {
		t.Fatalf("fetches within the interval = %d, want 1 (the initial load)", cs.hitCount())
	}

	clk.advance(minRefreshInterval + time.Second)
	for i := 0; i < 10; i++ {
		_ = v.Verify(ctx, sign(t, key, "attacker", claimsAt(clk.now())))
	}
	if cs.hitCount() != 2 {
		t.Errorf("fetches after the interval = %d, want 2", cs.hitCount())
	}
	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Errorf("known kid rejected after the attack: %v", err)
	}
}

func TestVerify_ConcurrentRefreshesShareOneFetch(t *testing.T) {
	key, cert := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert}, block: make(chan struct{}), started: make(chan struct{}, 20)}
	v := newVerifier(aud, cs.start(t), time.Now)
	token := sign(t, key, "k1", validClaims())

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- v.Verify(context.Background(), token)
		}()
	}
	<-cs.started
	time.Sleep(50 * time.Millisecond) // let the others queue on the flight
	close(cs.block)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Verify = %v", err)
		}
	}
	if cs.hitCount() != 1 {
		t.Errorf("certificate fetches = %d, want 1", cs.hitCount())
	}
}

func TestVerify_CachedKidDoesNotWaitBehindAFetch(t *testing.T) {
	key, cert := newKey(t)
	clk := newClock()
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), clk.now)
	ctx := context.Background()
	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatal(err)
	}

	block := make(chan struct{})
	cs.mu.Lock()
	cs.block, cs.started = block, make(chan struct{}, 1)
	started := cs.started
	cs.mu.Unlock()
	clk.advance(minRefreshInterval + time.Second)
	slow := make(chan error, 1)
	go func() { slow <- v.Verify(ctx, sign(t, key, "unknown", claimsAt(clk.now()))) }()
	<-started

	done := make(chan error, 1)
	go func() { done <- v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("cached kid: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("a cached kid waited behind the certificate fetch")
	}
	close(block)
	if err := <-slow; !errors.Is(err, ErrInvalidToken) {
		t.Errorf("unknown kid: err = %v", err)
	}
}

func TestVerify_StaleKeysSurviveAFailedRefresh(t *testing.T) {
	key, cert := newKey(t)
	clk := newClock()
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), clk.now)
	ctx := context.Background()
	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatal(err)
	}

	cs.mu.Lock()
	cs.fail = true
	cs.mu.Unlock()
	clk.advance(2 * time.Hour) // past max-age=3600
	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatalf("expired cache + failed refresh rejected a known kid: %v", err)
	}
	if err := v.Verify(ctx, sign(t, key, "k1", claimsAt(clk.now()))); err != nil {
		t.Fatal(err)
	}
	if cs.hitCount() != 2 {
		t.Errorf("certificate fetches = %d, want 2 (failed refresh retried at most once per interval)", cs.hitCount())
	}
}

func TestVerify_FailedFirstLoadReturnsAnError(t *testing.T) {
	key, _ := newKey(t)
	cs := &certServer{fail: true}
	v := newVerifier(aud, cs.start(t), time.Now)
	if err := v.Verify(context.Background(), sign(t, key, "k1", validClaims())); err == nil {
		t.Error("Verify succeeded without any certificate")
	}
}

func TestVerify_AudienceAsArray(t *testing.T) {
	key, cert := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), time.Now)
	c := validClaims()
	c["aud"] = []string{"x", aud}
	if err := v.Verify(context.Background(), sign(t, key, "k1", c)); err != nil {
		t.Errorf("array audience containing ours rejected: %v", err)
	}
}
