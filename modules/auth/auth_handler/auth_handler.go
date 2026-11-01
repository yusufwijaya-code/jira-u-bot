package auth_handler

import "github.com/gin-gonic/gin"

type AuthHandler interface {
	// LoginPage renders the app's own landing page (Sign in with Google
	// button) without touching Google at all — distinct from Login, which
	// actually kicks off the OAuth round-trip once the button is clicked.
	LoginPage(c *gin.Context)
	Login(c *gin.Context)
	Callback(c *gin.Context)
	Logout(c *gin.Context)
}
