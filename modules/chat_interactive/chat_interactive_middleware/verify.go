package chat_interactive_middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/idtoken"
)

// TokenVerifier validates a Google-signed OIDC bearer token against an
// expected audience. Defined as an interface (not a direct idtoken.Validate
// call in the middleware) because a real Google-signed token cannot be
// constructed in a unit test — tests inject a fake instead.
type TokenVerifier interface {
	Verify(ctx context.Context, token, audience string) error
}

type googleTokenVerifier struct{}

// NewGoogleTokenVerifier returns the production TokenVerifier, backed by
// Google's own idtoken package (handles fetching/caching Google's public
// certs internally — no manual JWKS handling needed).
func NewGoogleTokenVerifier() TokenVerifier {
	return &googleTokenVerifier{}
}

func (v *googleTokenVerifier) Verify(ctx context.Context, token, audience string) error {
	_, err := idtoken.Validate(ctx, token, audience)
	return err
}

// VerifyChatRequest implements Tech Spec AC1.1/AC1.2: every request to the
// interactive bot endpoint must carry a valid Google-signed bearer token
// whose audience matches this endpoint's own URL, or it's rejected with 401
// before any command logic runs. Verification failures are never logged
// with the token itself (Tech Spec Observability — avoid logging secrets).
func VerifyChatRequest(verifier TokenVerifier, audience string) gin.HandlerFunc {
	const bearerPrefix = "Bearer "
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, bearerPrefix)

		if err := verifier.Verify(c.Request.Context(), token, audience); err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
}
