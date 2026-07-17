package app

import (
	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/project-hawaii/app/api"
	"github.com/z46-dev/project-hawaii/config"
)

func Init(log *golog.Logger) (err error) {
	var app *fiber.App = fiber.New()

	app.Use(securityHeaders)
	api.Init(app, log)

	log.Infof("Starting the web server on %s\n", config.Conf.WebServer.BindAddress)
	err = app.Listen(config.Conf.WebServer.BindAddress)
	return
}
