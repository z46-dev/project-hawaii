package v1

import "github.com/gofiber/fiber/v3"

// Health reports the API's health status.
func Health(ctx fiber.Ctx) (err error) {
	err = ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"nominal": true,
	})

	return
}
