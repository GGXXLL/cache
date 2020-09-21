package persistence

import (
	"github.com/gin-contrib/cache/utils"
	"github.com/go-redis/redis"
	"strings"
	"time"
)

// GoRedisStore represents the cache with redis persistence
type GoRedisStore struct {
	cli               redis.UniversalClient
	defaultExpiration time.Duration
}

// NewRedisCache returns a GoRedisStore
// until redigo supports sharding/clustering, only one host will be in hostList
func NewGoRedisCache(host string, password string, defaultExpiration time.Duration) *GoRedisStore {
	addrs := strings.Split(host, ",")
	cli := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:          addrs,
		DialTimeout:    time.Duration(2) * time.Second,
		ReadTimeout:    time.Duration(2) * time.Second,
		IdleTimeout:    time.Duration(3) * time.Minute,
		PoolSize:       2,
		PoolTimeout:    30 * time.Second,
		ReadOnly:       true,
		RouteByLatency: true,
		Password:       password,
	})
	cmd := cli.Ping()
	if cmd.Err() != nil {
		panic(cmd.Err())
	}

	return &GoRedisStore{cli, defaultExpiration}
}

func NewGoRedisCacheWithOption(opt *redis.UniversalOptions, defaultExpiration time.Duration) *GoRedisStore {
	cli := redis.NewUniversalClient(opt)
	cmd := cli.Ping()
	if cmd.Err() != nil {
		panic(cmd.Err())
	}
	return &GoRedisStore{cli, defaultExpiration}
}

func NewGoRedisCacheWithClient(cli redis.UniversalClient, defaultExpiration time.Duration) *GoRedisStore {
	return &GoRedisStore{cli, defaultExpiration}
}

// Set (see CacheStore interface)
func (c *GoRedisStore) Set(key string, value interface{}, expires time.Duration) error {
	b, err := utils.Serialize(value)
	if err != nil {
		return err
	}
	err = c.cli.Set(key, b, expires).Err()
	return err
}

// Add (see CacheStore interface)
func (c *GoRedisStore) Add(key string, value interface{}, expires time.Duration) error {
	if !c.exists(key) {
		return ErrNotStored
	}
	err := c.Set(key, value, expires)
	return err
}

// Replace (see CacheStore interface)
func (c *GoRedisStore) Replace(key string, value interface{}, expires time.Duration) error {
	if !c.exists(key) {
		return ErrNotStored
	}
	err := c.Set(key, value, expires)
	return err
}

// Get (see CacheStore interface)
func (c *GoRedisStore) Get(key string, ptrValue interface{}) error {
	raw, err := c.cli.Get(key).Result()
	if err == redis.Nil {
		return ErrNotStored
	}
	if err != nil {
		return err
	}
	return utils.Deserialize([]byte(raw), ptrValue)
}

func (c *GoRedisStore) exists(key string) bool {
	ret, _ := c.cli.Exists(key).Result()
	return ret == 1
}

// Delete (see CacheStore interface)
func (c *GoRedisStore) Delete(key string) error {
	err := c.cli.Del(key).Err()
	return err
}

// Increment (see CacheStore interface)
func (c *GoRedisStore) Increment(key string, delta uint64) (uint64, error) {
	val, err := c.cli.IncrBy(key, int64(delta)).Result()
	if err != nil {
		return 0, err
	}
	return uint64(val), nil
}

// Decrement (see CacheStore interface)
func (c *GoRedisStore) Decrement(key string, delta uint64) (newValue uint64, err error) {
	val, err := c.cli.DecrBy(key, int64(delta)).Result()
	if err != nil {
		return 0, err
	}
	return uint64(val), nil
}

// FlushAll (see CacheStore interface)
func (c *GoRedisStore) Flush() error {
	err := c.cli.FlushAll().Err()
	return err
}