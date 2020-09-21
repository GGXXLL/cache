package persistence

import (
	"net"
	"testing"
	"time"
)

var newGoRedisStore = func(t *testing.T, defaultExpiration time.Duration) CacheStore {
	c, err := net.Dial("tcp", redisTestServer)
	if err == nil {
		c.Write([]byte("flush_all\r\n"))
		c.Close()
		redisCache := NewGoRedisCache(redisTestServer, "", defaultExpiration)
		//redisCache.Flush()
		return redisCache
	}
	t.Errorf("couldn't connect to redis on %s", redisTestServer)
	t.FailNow()
	panic("")
}

func TestGoRedisCache_TypicalGetSet(t *testing.T) {
	typicalGetSet(t, newGoRedisStore)
}

func TestGoRedisCache_IncrDecr(t *testing.T) {
	incrDecr(t, newGoRedisStore)
}

func TestGoRedisCache_Expiration(t *testing.T) {
	expiration(t, newGoRedisStore)
}

func TestGoRedisCache_EmptyCache(t *testing.T) {
	emptyCache(t, newGoRedisStore)
}

func TestGoRedisCache_Replace(t *testing.T) {
	testReplace(t, newGoRedisStore)
}

func TestGoRedisCache_Add(t *testing.T) {
	testAdd(t, newGoRedisStore)
}
