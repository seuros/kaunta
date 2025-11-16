package cli

import "github.com/gofiber/fiber/v3"

// createFiberConfig returns Fiber configuration.
func createFiberConfig(appName string) fiber.Config {
	return fiber.Config{
		AppName: appName,
		// Use X-Forwarded-For to get real client IP behind reverse proxy
		ProxyHeader: fiber.HeaderXForwardedFor,
	}
}
