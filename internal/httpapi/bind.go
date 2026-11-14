package httpapi

import "github.com/gin-gonic/gin"

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, "invalid_request", "Malformed JSON body or missing field.", nil)
		return false
	}
	return true
}

func noRoute(c *gin.Context) {
	fail(c, "not_found", "Route not found.", nil)
}
