package redis

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/go-redis/redis"
)

var (
	redisClient *redis.Client
	scriptHash  string
)

const scriptTokensBucket = `
local tokens_key = KEYS[1]
local timestamp_key = KEYS[2]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local fill_time = capacity/rate
local ttl = math.floor(fill_time*2)
local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil then
    last_tokens = capacity
end
local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil then
    last_refreshed = 0
end
local delta = math.max(0, now-last_refreshed)
local filled_tokens = math.min(capacity, last_tokens+(delta*rate))
local allowed = filled_tokens >= requested
local new_tokens = filled_tokens
if allowed then
    new_tokens = filled_tokens - requested
end
redis.call("setex", tokens_key, 2, new_tokens)
redis.call("setex", timestamp_key, 2, now)
return { allowed, new_tokens }
`
const scriptWindowLog = `
local key_rate = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local clearBefore = now - window
redis.call('ZREMRANGEBYSCORE', key_rate, 0, clearBefore)
local already_sent = tonumber(redis.call('ZCARD', key_rate))
if already_sent < limit then
redis.call('ZADD', key_rate, now, now)
end

redis.call('EXPIRE', key_rate, window)
local response=0
if already_sent<limit then
response=1
end
redis.call('set', "already_sent", already_sent)
redis.call('set', "limit", limit)
redis.call('set', "response", response)
return {response, already_sent}
`

// Client indicates the redis client of the rate limiter.
func Client() *redis.Client {
	return redisClient
}

// SetRedis sets the redis client.
func SetRedis(config *ConfigRedis) error {
	if config == nil {
		return errors.New("redis config is empty")
	}

	redisClient = newRedisClient(*config)
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	go func() {
		timer := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-timer.C:
				loadScript()
			}
		}
	}()
	return loadScript()
}

func loadScript() error {
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	scriptHash = fmt.Sprintf("%x", sha1.Sum([]byte(scriptWindowLog)))
	exists, err := redisClient.ScriptExists(scriptHash).Result()
	if err != nil {
		return err
	}

	// load script when missing.
	if !exists[0] {
		_, err := redisClient.ScriptLoad(scriptWindowLog).Result()
		if err != nil {
			return err
		}
	}
	return nil
}

type Window int64

const Inf = Window(math.MaxInt64)

// A Limiter controls how frequently events are allowed to happen.
type Limiter struct {
	window Window
	limit  int
	key    string
}

func NewLimiter(w Window, l int, key string) *Limiter {
	return &Limiter{
		window: w,
		limit:  l,
		key:    key,
	}
}

// Every converts a minimum time interval between events to a Limit.
func Every(interval time.Duration) Window {
	if interval <= 0 {
		return Inf
	}
	return Window(interval)
}

func (lim *Limiter) Allow() bool {
	if redisClient == nil {
		return false
	}

	results, err := redisClient.EvalSha(
		scriptHash,
		[]string{lim.key},
		time.Now().UnixNano(),
		float64(lim.window),
		lim.limit,
	).Result()
	if err != nil {
		log.Println("fail to call rate limit: ", err)
		return false
	}

	rs, ok := results.([]interface{})
	if ok {
		//println(rs[1].(int64))
		return rs[0].(int64) == 1
	}

	log.Println("fail to transform results")
	return false
}
