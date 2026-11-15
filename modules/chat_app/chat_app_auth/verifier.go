// Package chat_app_auth verifies the bearer token Google Chat puts on every
// request to the Chat App's HTTP endpoint. With "Authentication Audience =
// Project Number" the token is a JWT signed by chat@system.gserviceaccount.com
// whose aud is the GCP project number (Tech Spec "Nudge sebagai Google Chat
// App", AC2.1). idtoken.Validate does not apply: it checks Google's OIDC
// keys, not this service account's.
package chat_app_auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ChatIssuer      = "chat@system.gserviceaccount.com"
	defaultCertsURL = "https://www.googleapis.com/service_accounts/v1/metadata/x509/chat@system.gserviceaccount.com"
	clockSkew       = 60 * time.Second
	defaultCertTTL  = time.Hour

	// MaxTokenBytes caps the bearer token before any decoding; Google's
	// tokens are about 1KB.
	MaxTokenBytes = 8 << 10
	// maxCertsBytes caps the certificate document read from Google.
	maxCertsBytes = 1 << 20
	// minRefreshInterval spaces certificate fetches out: an unknown kid, an
	// expired cache or a failed refresh triggers at most one fetch per
	// interval, so an unauthenticated caller can't make us hammer googleapis.
	minRefreshInterval = 60 * time.Second
)

var ErrInvalidToken = errors.New("chat_app_auth: invalid token")

// errRefreshSkipped means a refresh was rate-limited (minRefreshInterval).
var errRefreshSkipped = errors.New("chat_app_auth: certificate refresh rate-limited")

var maxAgePattern = regexp.MustCompile(`max-age=(\d+)`)

// flight is one in-progress certificate fetch that concurrent callers share.
type flight struct {
	done chan struct{}
	err  error
}

type Verifier struct {
	audience string
	certsURL string
	client   *http.Client
	now      func() time.Time

	// mu guards keys and expires. It is never held across the HTTP fetch,
	// so verifying a cached kid never waits behind a refresh.
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	expires time.Time

	// fetchMu guards inflight and lastAttempt (a stdlib singleflight).
	fetchMu     sync.Mutex
	inflight    *flight
	lastAttempt time.Time
}

func NewVerifier(audience string) *Verifier {
	return newVerifier(audience, defaultCertsURL, time.Now)
}

func newVerifier(audience, certsURL string, now func() time.Time) *Verifier {
	return &Verifier{audience: audience, certsURL: certsURL, client: &http.Client{Timeout: 10 * time.Second}, now: now}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// audience accepts the JWT "aud" claim as a string or an array of strings.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

type jwtClaims struct {
	Iss string   `json:"iss"`
	Aud audience `json:"aud"`
	Exp int64    `json:"exp"`
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidToken, fmt.Sprintf(format, args...))
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Verify checks signature, issuer, audience and expiry of token. Values
// taken from the token are %q-quoted in errors because they get logged.
func (v *Verifier) Verify(ctx context.Context, token string) error {
	if len(token) > MaxTokenBytes {
		return invalid("token too large (%d bytes)", len(token))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return invalid("malformed token")
	}
	var h jwtHeader
	if err := decodeSegment(parts[0], &h); err != nil {
		return invalid("header: %q", err.Error())
	}
	if h.Alg != "RS256" {
		return invalid("unexpected alg %q", h.Alg)
	}
	var c jwtClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return invalid("claims: %q", err.Error())
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return invalid("signature encoding: %q", err.Error())
	}
	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return invalid("bad signature")
	}
	if c.Iss != ChatIssuer {
		return invalid("issuer %q", c.Iss)
	}
	audOK := false
	for _, a := range c.Aud {
		if a == v.audience {
			audOK = true
		}
	}
	if !audOK {
		return invalid("audience %q", []string(c.Aud))
	}
	if v.now().After(time.Unix(c.Exp, 0).Add(clockSkew)) {
		return invalid("expired")
	}
	return nil
}

func (v *Verifier) cached(kid string) (key *rsa.PublicKey, known, fresh bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, known = v.keys[kid]
	return key, known, v.now().Before(v.expires)
}

// key returns the public key for kid. A fresh cache hit never waits. An
// unknown kid or an expired cache refreshes the certificates, at most once
// per minRefreshInterval; when that refresh fails or is skipped, the
// previous (stale) keys keep being used.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if k, known, fresh := v.cached(kid); known && fresh {
		return k, nil
	}
	refreshErr := v.refreshShared(ctx)
	if k, known, _ := v.cached(kid); known {
		return k, nil
	}
	if refreshErr != nil && !errors.Is(refreshErr, errRefreshSkipped) {
		return nil, refreshErr
	}
	return nil, invalid("unknown kid %q", kid)
}

// refreshShared runs one certificate fetch for all concurrent callers and
// starts at most one fetch per minRefreshInterval.
func (v *Verifier) refreshShared(ctx context.Context) error {
	v.fetchMu.Lock()
	if f := v.inflight; f != nil {
		v.fetchMu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	now := v.now()
	if !v.lastAttempt.IsZero() && now.Sub(v.lastAttempt) < minRefreshInterval {
		v.fetchMu.Unlock()
		return errRefreshSkipped
	}
	v.lastAttempt = now
	f := &flight{done: make(chan struct{})}
	v.inflight = f
	v.fetchMu.Unlock()

	// Always release waiters, even if refresh panics.
	defer func() {
		if r := recover(); r != nil {
			f.err = fmt.Errorf("chat_app_auth: certificate refresh panicked: %v", r)
		}
		v.fetchMu.Lock()
		v.inflight = nil
		v.fetchMu.Unlock()
		close(f.done)
	}()

	// Detached from the caller: the fetch serves every waiter, and the
	// client timeout bounds it.
	f.err = v.refresh(context.WithoutCancel(ctx))
	return f.err
}

// refresh fetches the certificates and swaps them in. On any error the
// current keys stay.
func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return fmt.Errorf("chat_app_auth: build certs request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("chat_app_auth: fetch certs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chat_app_auth: fetch certs: status %d", resp.StatusCode)
	}
	var raw map[string]string
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCertsBytes)).Decode(&raw); err != nil {
		return fmt.Errorf("chat_app_auth: decode certs: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(raw))
	for kid, certPEM := range raw {
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			keys[kid] = pub
		}
	}
	if len(keys) == 0 {
		return errors.New("chat_app_auth: certs document has no usable RSA certificate")
	}
	ttl := defaultCertTTL
	if m := maxAgePattern.FindStringSubmatch(resp.Header.Get("Cache-Control")); m != nil {
		if secs, err := strconv.Atoi(m[1]); err == nil && secs > 0 {
			ttl = time.Duration(secs) * time.Second
		}
	}
	v.mu.Lock()
	v.keys = keys
	v.expires = v.now().Add(ttl)
	v.mu.Unlock()
	return nil
}
