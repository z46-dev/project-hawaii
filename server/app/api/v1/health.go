package v1

import "github.com/gofiber/fiber/v3"

// Returns the health status of the API. This endpoint is used for health checks and monitoring.
// Currently it just returns 200 OK and JSON {"ok": true}. In the future, it may include more detailed health information.
func Health(ctx fiber.Ctx) (err error) {
	err = ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"nominal": true,
	})

	return
}
