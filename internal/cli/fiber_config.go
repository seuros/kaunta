package cli

import "github.com/gofiber/fiber/v3"

// createFiberConfig returns Fiber configuration.
func createFiberConfig(appName string) fiber.Config {
	return fiber.Config{
		AppName: appName,
		// Trust reverse proxies to properly detect HTTPS via X-Forwarded-Proto
		ProxyHeader: fiber.HeaderXForwardedProto,
		TrustProxy:  true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: []string{
				"10.0.0.0/8",     // private networks
				"172.16.0.0/12",  // private networks
				"192.168.0.0/16", // private networks
			},
			Loopback: true, // Trust 127.0.0.0/8 and ::1
		},
	}
}
