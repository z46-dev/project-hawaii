package app

import (
	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/project-hawaii/app/api"
	"github.com/z46-dev/project-hawaii/config"
)

// Init creates the HTTP application and starts its listener.
func Init(log *golog.Logger) (err error) {
	var app *fiber.App = fiber.New(fiber.Config{
		TrustProxy:         true,
		TrustProxyConfig:   fiber.TrustProxyConfig{Proxies: config.Conf.WebServer.TrustedProxies},
		ProxyHeader:        fiber.HeaderXForwardedFor,
		EnableIPValidation: true,
	})

	app.Use(securityHeaders)
	api.Init(app, log)

	log.Infof("Starting the web server on %s\n", config.Conf.WebServer.BindAddress)
	err = app.Listen(config.Conf.WebServer.BindAddress)
	return
}
