package cache

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
)

const (
	MiddlewareKey = "gincontrib.cache"
)

var (
	PageCachePrefix = "gincontrib.page.cache"
)

type cache struct {
	store            persistence.CacheStore
	excludeQueryArgs []string // just support GET request
}

func (ch *cache) SetExcludeQueryArgs(values ...string) {
	ch.excludeQueryArgs = append(ch.excludeQueryArgs, values...)
}

func (ch *cache) parseUrl(u *url.URL) *url.URL {
	if len(ch.excludeQueryArgs) > 0 {
		q := u.Query()
		for _, v := range ch.excludeQueryArgs {
			q.Del(v)
		}
		u.RawQuery = q.Encode()
	}
	return u
}

func (ch *cache) validResponse(method string, code int) bool {
	if method != "GET" || code != 0 {
		return false
	}
	return true
}

func NewCache(store persistence.CacheStore) *cache {
	return &cache{
		store: store,
	}
}

func NewMemoryCache(expire time.Duration) *cache {
	store := persistence.NewInMemoryStore(expire)
	return NewCache(store)
}

func NewRedisCache(host string, password string, defaultExpiration time.Duration) *cache {
	store := persistence.NewRedisCache(host, password, defaultExpiration)
	return NewCache(store)
}

func NewGoRedisCache(host string, password string, defaultExpiration time.Duration) *cache {
	store := persistence.NewGoRedisStore(host, password, defaultExpiration)
	return NewCache(store)
}


func NewMemcached(hostList []string, defaultExpiration time.Duration) *cache {
	store := persistence.NewMemcachedStore(hostList, defaultExpiration)
	return NewCache(store)
}

type responseCache struct {
	Status int
	Header http.Header
	Data   []byte
}

// RegisterResponseCacheGob registers the responseCache type with the encoding/gob package
func RegisterResponseCacheGob() {
	gob.Register(responseCache{})
}

type cachedWriter struct {
	gin.ResponseWriter
	status  int
	written bool
	store   persistence.CacheStore
	expire  time.Duration
	key     string
}

var _ gin.ResponseWriter = &cachedWriter{}

// CreateKey creates a package specific key for a given string
func CreateKey(u string) string {
	return urlEscape(PageCachePrefix, u)
}

func urlEscape(prefix string, u string) string {
	key := url.QueryEscape(u)
	if len(key) > 200 {
		h := sha1.New()
		io.WriteString(h, u)
		key = string(h.Sum(nil))
	}
	var buffer bytes.Buffer
	buffer.WriteString(prefix)
	buffer.WriteString(":")
	buffer.WriteString(key)
	return buffer.String()
}

func newCachedWriter(store persistence.CacheStore, expire time.Duration, writer gin.ResponseWriter, key string) *cachedWriter {
	return &cachedWriter{writer, 0, false, store, expire, key}
}

func (w *cachedWriter) WriteHeader(code int) {
	w.status = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *cachedWriter) Status() int {
	return w.ResponseWriter.Status()
}

func (w *cachedWriter) Written() bool {
	return w.ResponseWriter.Written()
}

func (w *cachedWriter) Write(data []byte) (int, error) {
	ret, err := w.ResponseWriter.Write(data)
	if err == nil {
		store := w.store
		var cache responseCache
		if err := store.Get(w.key, &cache); err == nil {
			data = append(cache.Data, data...)
		}

		//cache responses with a status code < 300
		if w.Status() < 300 {
			val := responseCache{
				w.Status(),
				w.Header(),
				data,
			}
			err = store.Set(w.key, val, w.expire)
			if err != nil {
				// need logger
			}
		}
	}
	return ret, err
}

func (w *cachedWriter) WriteString(data string) (n int, err error) {
	ret, err := w.ResponseWriter.WriteString(data)
	//cache responses with a status code < 300
	if err == nil && w.Status() < 300 {
		store := w.store
		val := responseCache{
			w.Status(),
			w.Header(),
			[]byte(data),
		}
		store.Set(w.key, val, w.expire)
	}
	return ret, err
}

// Cache Middleware
func (ch *cache) Cache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(MiddlewareKey, ch.store)
		c.Next()
	}
}

func (ch *cache) SiteCache(expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var repCache responseCache
		u := ch.parseUrl(c.Request.URL)
		key := CreateKey(u.RequestURI())
		if err := ch.store.Get(key, &repCache); err != nil {
			c.Next()
		} else {
			c.Writer.WriteHeader(repCache.Status)
			for k, vals := range repCache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(repCache.Data)
			c.Abort()
		}
	}
}

// CachePage Decorator
func (ch *cache) CachePage(expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var repCache responseCache
		u := ch.parseUrl(c.Request.URL)
		key := CreateKey(u.RequestURI())
		if err := ch.store.Get(key, &repCache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(ch.store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()
			// Drop caches of aborted contexts
			if c.IsAborted() {
				ch.store.Delete(key)
			}
		} else {
			c.Writer.WriteHeader(repCache.Status)
			for k, vals := range repCache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(repCache.Data)
			c.Abort()
		}
	}
}

// CachePageAtomic Decorator
func (ch *cache) CachePageAtomic(expire time.Duration) gin.HandlerFunc {
	var m sync.Mutex
	p := ch.CachePage(expire)
	return func(c *gin.Context) {
		m.Lock()
		defer m.Unlock()
		p(c)
	}
}

// CachePageWithoutQuery add ability to ignore GET query parameters.
func (ch *cache) CachePageWithoutQuery(expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var repCache responseCache

		key := CreateKey(c.Request.URL.Path)
		if err := ch.store.Get(key, &repCache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(ch.store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()
		} else {
			c.Writer.WriteHeader(repCache.Status)
			for k, vals := range repCache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(repCache.Data)
			c.Abort()
		}
	}
}

func (ch *cache) CachePageWithoutHeader(expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var repCache responseCache
		u := ch.parseUrl(c.Request.URL)
		key := CreateKey(u.RequestURI())
		if err := ch.store.Get(key, &repCache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(ch.store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()

			// Drop caches of aborted contexts
			if c.IsAborted() {
				ch.store.Delete(key)
			}
		} else {
			c.Writer.WriteHeader(repCache.Status)
			c.Writer.Write(repCache.Data)
			c.Abort()
		}
	}
}
