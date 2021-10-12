package main

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
	"velox/rateLimiter/redis"
)

func init() {
	_ = redis.SetRedis(&redis.ConfigRedis{
		Host: "127.0.0.1",
		Port: 6379,
		Auth: "",
	})
}

func TestTokenBucket_Allow(t *testing.T) {
	go testToken(t)
	select {
	case <-time.After(5 * time.Second):
		fmt.Println("Time Out!")
		return
	}

}
func testToken(t *testing.T) {
	for range time.Tick(time.Second) {
		limiter := redis.NewLimiter(redis.Every(time.Second*2), 1, "r")
		al := limiter.Allow()
		fmt.Println(al)
	}
}

func testTokenUrl(t *testing.T) {
	for range time.Tick(time.Second) {
		al, _ := http.Get("http://localhost:4000")
		b, _ := io.ReadAll(al.Body)

		fmt.Println(al.StatusCode, string(b))
	}
}
