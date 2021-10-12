package main

import (
	"github.com/gofiber/fiber/v2"
	"log"
	"os"
	"time"
	"velox/rateLimiter/redis"
)

type Response struct {
	NameServer string `json:"nameServer"`
	Status     int    `json:"status"`
}

func main() {
	app := fiber.New()
	err := redis.SetRedis(&redis.ConfigRedis{
		Host: "test-redis",
		Port: 6379,
		Auth: "",
	})
	if err != nil {
		log.Fatal(err)
	}
	app.Get("", rateLimiter(time.Second*2, 1), func(ctx *fiber.Ctx) error {
		return ctx.JSON(Response{NameServer: os.Getenv("NAME"), Status: fiber.StatusOK})
	})
	app.Listen(":4000")
}

func rateLimiter(expiration time.Duration, limit int) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		limiter := redis.NewLimiter(redis.Every(expiration), limit, ctx.IP())
		if limiter.Allow() {
			return ctx.Next()
		}
		return ctx.Status(fiber.StatusTooManyRequests).JSON(Response{NameServer: os.Getenv("NAME"), Status: fiber.StatusTooManyRequests})
	}

}
