package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/golog"
	v1 "github.com/z46-dev/project-hawaii/app/api/v1"
)

func Init(app *fiber.App, _log *golog.Logger) {
	var apiGroup fiber.Router = app.Group("/api")

	v1.Init(apiGroup, _log)
}
