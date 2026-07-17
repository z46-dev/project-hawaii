package v1

import (
	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/golog"
)

var log *golog.Logger

func Init(apiGroup fiber.Router, _log *golog.Logger) {
	var v1 fiber.Router = apiGroup.Group("/v1")
	log = _log

	v1.Get("/health", Health)
}
