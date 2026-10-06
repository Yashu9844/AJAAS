package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const corsAllowedHeaders = "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Request-ID, X-Platform-Key"

// CORS handles Cross-Origin Resource Sharing. allowedOrigins is an explicit allow-list; a single "*" entry allows any
// origin (development only; credentials are then NOT allowed, per the CORS spec). An empty list disables CORS headers.
// Tenant subdomain origins can be allowed with a "*.example.com" wildcard entry.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowAll := false
	exact := map[string]bool{}
	var suffixes []string
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		switch {
		case o == "*":
			allowAll = true
		case strings.Contains(o, "://*."):
			parts := strings.SplitN(o, "://*.", 2)
			suffixes = append(suffixes, parts[0]+"://"+"."+parts[1]) // e.g. "https://.example.com"
		case o != "":
			exact[o] = true
		}
	}
	match := func(origin string) bool {
		if exact[origin] {
			return true
		}
		for _, sfx := range suffixes {
			scheme := sfx[:strings.Index(sfx, "://")+3]
			dom := sfx[len(scheme):] // ".example.com"
			if strings.HasPrefix(origin, scheme) && strings.HasSuffix(strings.SplitN(origin[len(scheme):], ":", 2)[0], dom) {
				return true
			}
		}
		return false
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			h := c.Writer.Header()
			h.Add("Vary", "Origin")
			switch {
			case allowAll:
				h.Set("Access-Control-Allow-Origin", "*")
			case match(origin):
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if h.Get("Access-Control-Allow-Origin") != "" {
				h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				h.Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
				h.Set("Access-Control-Max-Age", "600")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
