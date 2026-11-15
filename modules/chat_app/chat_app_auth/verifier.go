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
)

var ErrInvalidToken = errors.New("chat_app_auth: invalid token")

var maxAgePattern = regexp.MustCompile(`max-age=(\d+)`)

type Verifier struct {
	audience string
	certsURL string
	client   *http.Client
	now      func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
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

// Verify checks signature, issuer, audience and expiry of token.
func (v *Verifier) Verify(ctx context.Context, token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return invalid("malformed token")
	}
	var h jwtHeader
	if err := decodeSegment(parts[0], &h); err != nil {
		return invalid("header: %v", err)
	}
	if h.Alg != "RS256" {
		return invalid("unexpected alg %q", h.Alg)
	}
	var c jwtClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return invalid("claims: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return invalid("signature encoding: %v", err)
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
		return invalid("audience %v", []string(c.Aud))
	}
	if v.now().After(time.Unix(c.Exp, 0).Add(clockSkew)) {
		return invalid("expired")
	}
	return nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && v.now().Before(v.expires) {
		return k, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, invalid("unknown kid %q", kid)
}

// refresh reloads the certificates; the caller holds v.mu.
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
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
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
	ttl := defaultCertTTL
	if m := maxAgePattern.FindStringSubmatch(resp.Header.Get("Cache-Control")); m != nil {
		if secs, err := strconv.Atoi(m[1]); err == nil && secs > 0 {
			ttl = time.Duration(secs) * time.Second
		}
	}
	v.keys = keys
	v.expires = v.now().Add(ttl)
	return nil
}
