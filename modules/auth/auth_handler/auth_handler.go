package auth_handler

import "github.com/gin-gonic/gin"

type AuthHandler interface {
	// Me returns the current session's email and admin status as JSON —
	// used by the Angular frontend to decide whether to show the Login
	// Screen or the Dashboard Shell. Mounted behind AuthMiddlewareAPI.
	Me(c *gin.Context)
	Login(c *gin.Context)
	Callback(c *gin.Context)
	Logout(c *gin.Context)
}
