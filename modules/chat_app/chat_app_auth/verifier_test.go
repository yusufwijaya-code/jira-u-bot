package chat_app_auth

import (
	"context"
	"crypto"
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

func sign(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

type certServer struct {
	mu    sync.Mutex
	certs map[string]string
	hits  int
}

func (s *certServer) start(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.hits++
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(s.certs)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const aud = "275006361614"

func validClaims() map[string]any {
	return map[string]any{"iss": ChatIssuer, "aud": aud, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix()}
}

func TestVerify(t *testing.T) {
	key, cert := newKey(t)
	other, _ := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert}}
	v := newVerifier(aud, cs.start(t), time.Now)
	ctx := context.Background()

	if err := v.Verify(ctx, sign(t, key, "k1", validClaims())); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

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
		"malformed":     "abc.def",
		"empty":         "",
	}
	for name, token := range cases {
		if err := v.Verify(ctx, token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v, want ErrInvalidToken", name, err)
		}
	}
}

func TestVerify_RefetchesCertsForANewKid(t *testing.T) {
	key1, cert1 := newKey(t)
	key2, cert2 := newKey(t)
	cs := &certServer{certs: map[string]string{"k1": cert1}}
	v := newVerifier(aud, cs.start(t), time.Now)
	ctx := context.Background()

	if err := v.Verify(ctx, sign(t, key1, "k1", validClaims())); err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	cs.certs = map[string]string{"k2": cert2}
	cs.mu.Unlock()
	if err := v.Verify(ctx, sign(t, key2, "k2", validClaims())); err != nil {
		t.Fatalf("rotated key rejected: %v", err)
	}
	if err := v.Verify(ctx, sign(t, key1, "k1", validClaims())); err != nil && !strings.Contains(err.Error(), "kid") {
		t.Fatalf("unexpected error: %v", err)
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
