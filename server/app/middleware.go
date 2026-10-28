package app

import "github.com/gofiber/fiber/v3"

// securityHeaders adds response headers that constrain browser behavior.
func securityHeaders(c fiber.Ctx) (err error) {
	c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "same-origin")
	err = c.Next()
	return
}
