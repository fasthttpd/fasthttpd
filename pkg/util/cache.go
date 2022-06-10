package util

import (
	"hash"
	"hash/fnv"
	"sync"
	"time"
)

var hash64Pool = sync.Pool{
	New: func() interface{} {
		return fnv.New64()
	},
}

func acquireHash64() hash.Hash64 {
	return hash64Pool.Get().(hash.Hash64)
}

func releaseHash64(h hash.Hash64) {
	h.Reset()
	hash64Pool.Put(h)
}

type CacheKey uint64

func CacheKeyBytes(bs ...[]byte) CacheKey {
	h := acquireHash64()
	for _, b := range bs {
		h.Write(b)
	}
	key := h.Sum64()
	releaseHash64(h)
	return CacheKey(key)
}

func CacheKeyString(ss ...string) CacheKey {
	h := acquireHash64()
	for _, s := range ss {
		h.Write([]byte(s))
	}
	key := h.Sum64()
	releaseHash64(h)
	return CacheKey(key)
}

// Cache is an interface that defines accessor of the cache.
type Cache interface {
	Get(key CacheKey) interface{}
	Set(key CacheKey, value interface{})
	Del(key CacheKey)
	Len() int
	// OnRelease sets a callback that will be called on the key is released.
	OnRelease(cb func(key CacheKey, value interface{}))
}

const (
	// defaultExpire is 5 * 60 * 1000 ms (5 min)
	defaultExpire = 5 * 60 * 1000
	// defaultInterval is 60 * 1000 ms (1 min)
	defaultInterval = 60 * 1000
)

var (
	expireCacheNow = func() int64 {
		return time.Now().UnixMilli()
	}
	defaultOnRelease = func(key CacheKey, value interface{}) {}
)

type expireCacheValue struct {
	value interface{}
	peek  int64
}

type expireCache struct {
	store     map[CacheKey]*expireCacheValue
	expire    int64
	interval  int64
	next      int64
	mutex     sync.Mutex
	onRelease func(key CacheKey, value interface{})
}

var _ Cache = (*expireCache)(nil)

// NewExpireCache returns a new cache with the specified expire (ms) and
// default interval 1 min.
func NewExpireCache(expire int64) Cache {
	return NewExpireCacheInterval(expire, 0)
}

// NewExpireCacheInterval returns a new cache with the specified expire
// (ms) and interval (ms).
func NewExpireCacheInterval(expire, interval int64) Cache {
	if expire <= 0 {
		expire = defaultExpire
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	return &expireCache{
		store:     make(map[CacheKey]*expireCacheValue, 256),
		expire:    expire,
		interval:  interval,
		next:      expireCacheNow() + interval,
		onRelease: defaultOnRelease,
	}
}

// Get returns the value mapped to the specified key and extends its expiration.
func (c *expireCache) Get(key CacheKey) interface{} {
	v := c.store[key]
	if v == nil {
		return nil
	}
	now := expireCacheNow()
	v.peek = now
	c.expires(now)
	return v.value
}

// Set stores the value with key.
func (c *expireCache) Set(key CacheKey, value interface{}) {
	now := expireCacheNow()
	c.store[key] = &expireCacheValue{
		value: value,
		peek:  now,
	}
	c.expires(now)
}

func (c *expireCache) Del(key CacheKey) {
	if v, ok := c.store[key]; ok {
		delete(c.store, key)
		go c.onRelease(key, v.value)
	}
}

func (c *expireCache) Len() int {
	return len(c.store)
}

func (c *expireCache) OnRelease(cb func(key CacheKey, value interface{})) {
	if cb == nil {
		c.onRelease = defaultOnRelease
		return
	}
	c.onRelease = cb
}

func (c *expireCache) expires(now int64) {
	if now < c.next {
		return
	}
	c.mutex.Lock()
	if now < c.next {
		c.mutex.Unlock()
		return
	}
	c.next = now + c.interval
	c.mutex.Unlock()

	go func() {
		for k, v := range c.store {
			if now-v.peek > c.expire {
				delete(c.store, k)
				go c.onRelease(k, v.value)
			}
		}
	}()
}
