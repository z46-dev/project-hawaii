package v1

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/project-hawaii/app/api/v1/auth"
)

var log *golog.Logger

// Init registers version 1 API routes.
func Init(apiGroup fiber.Router, _log *golog.Logger) {
	var v1 fiber.Router = apiGroup.Group("/v1")
	log = _log

	var (
		loginRateLimit         fiber.Handler = authRateLimit(10, time.Minute, "login:")
		registrationRateLimit  fiber.Handler = authRateLimit(5, 24*time.Hour, "registration:")
		passwordResetRateLimit fiber.Handler = authRateLimit(10, time.Minute, "password-reset:")
	)

	v1.Get("/health", Health)
	v1.Get("/auth/realms", auth.Realms)
	v1.Post("/auth/login/:realm", loginRateLimit, auth.Login)
	v1.Post("/auth/register/:realm", registrationRateLimit, auth.RegisterLocal)
	v1.Post("/auth/password-reset/:token", passwordResetRateLimit, auth.ResetPassword)
	v1.Get("/auth/oidc/:realm", auth.OIDCLogin)
	v1.Get("/auth/oidc/:realm/callback", auth.OIDCCallback)
	v1.Get("/auth/oauth/:realm", auth.OAuthLogin)
	v1.Get("/auth/oauth/:realm/callback", auth.OAuthCallback)
	v1.Get("/auth/me", auth.Me)
	v1.Post("/auth/logout", auth.Logout)
}

// authRateLimit creates an IP-keyed limiter for an authentication endpoint.
func authRateLimit(max int, expiration time.Duration, prefix string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: expiration,
		KeyGenerator: func(ctx fiber.Ctx) string {
			return prefix + ctx.IP()
		},
	})
}
