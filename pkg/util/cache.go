package util

import (
	"sync"
	"time"
)

// Cache is an interface that defines accessor of the cache.
type Cache interface {
	Get(key string) interface{}
	Set(key string, value interface{})
}

const (
	// defaultExpire is 5 * 60 * 1000 ms (5 min)
	defaultExpire = 5 * 60 * 1000
	// defaultInterval is 60 * 1000 ms (1 min)
	defaultInterval = 60 * 1000
)

var expireCacheNow = func() int64 {
	return time.Now().UnixMilli()
}

type expireCacheValue struct {
	value interface{}
	peek  int64
}

type expireCache struct {
	store    map[string]*expireCacheValue
	expire   int64
	interval int64
	next     int64
	mutex    sync.Mutex
}

var _ Cache = (*expireCache)(nil)

// NewExpireCache returns a new cache with the specified expire (ms) and
// default interval 1 min.
func NewExpireCache(expire int64) Cache {
	return NewExpireCacheInterval(expire, defaultInterval)
}

// NewExpireCacheInterval returns a new cache with the specified expire
// (ms) and interval (ms).
func NewExpireCacheInterval(expire, interval int64) Cache {
	if expire <= 0 {
		expire = defaultExpire
	}
	return &expireCache{
		store:    map[string]*expireCacheValue{},
		expire:   expire,
		interval: interval,
		next:     expireCacheNow() + interval,
	}
}

// Get returns the value mapped to the specified key and extends its expiration.
func (c *expireCache) Get(key string) interface{} {
	v := c.store[key]
	if v == nil {
		return nil
	}
	now := expireCacheNow()
	v.peek = now
	c.notify(now)
	return v.value
}

// Set stores the value with key.
func (c *expireCache) Set(key string, value interface{}) {
	now := expireCacheNow()
	c.store[key] = &expireCacheValue{
		value: value,
		peek:  now,
	}
	c.notify(now)
}

func (c *expireCache) notify(now int64) {
	// NOTE: check befre lock
	if now < c.next {
		return
	}
	go func() {
		c.mutex.Lock()
		// NOTE: check in lock
		if now < c.next {
			c.mutex.Unlock()
			return
		}
		// NOTE: update c.next in lock
		c.next = now + c.interval
		c.mutex.Unlock()
		// NOTE: call expires without lock
		c.expires(now)
	}()
}

func (c *expireCache) expires(now int64) {
	for k, v := range c.store {
		if now-v.peek > c.expire {
			delete(c.store, k)
		}
	}
}
