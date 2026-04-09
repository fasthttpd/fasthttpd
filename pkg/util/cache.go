package util

import (
	"encoding/binary"
	"hash/maphash"
	"sync"
	"time"
)

// CacheKey is a 64-bit hashed cache key.
type CacheKey uint64

// cacheKeySeed is a single process-wide seed shared by every builder and
// shortcut helper so that equal inputs always produce equal CacheKeys.
// It is randomized at package init, which gives fasthttpd hash-flooding
// resistance against adversarial HTTP paths while still letting Set and
// Get within the same process agree on the same key.
var cacheKeySeed = maphash.MakeSeed()

// CacheKeyBuilder accumulates length-prefixed fields and produces a
// CacheKey that is safe against composition ambiguity — that is, the
// fields ("ab", "c") and ("a", "bc") produce different keys.
//
// Acquire from the pool with AcquireCacheKeyBuilder, call Write /
// WriteString as many times as needed, call Sum to finalize, then
// release it with ReleaseCacheKeyBuilder. Builders are not safe for
// concurrent use.
type CacheKeyBuilder struct {
	h      maphash.Hash
	// lenBuf is a 4-byte scratch reused by Write/WriteString to emit
	// the length prefix before each field. Keeping it on the struct
	// (which itself lives in a sync.Pool) means the prefix write stays
	// allocation-free across calls.
	lenBuf [4]byte
}

var cacheKeyBuilderPool = sync.Pool{
	New: func() any {
		b := &CacheKeyBuilder{}
		b.h.SetSeed(cacheKeySeed)
		return b
	},
}

// AcquireCacheKeyBuilder fetches a reusable builder from the pool. The
// returned builder has an empty hash state but keeps the shared seed,
// so successive Acquire/Release cycles produce consistent keys.
func AcquireCacheKeyBuilder() *CacheKeyBuilder {
	b := cacheKeyBuilderPool.Get().(*CacheKeyBuilder)
	b.h.Reset()
	return b
}

// ReleaseCacheKeyBuilder returns a builder to the pool. After calling
// this, the caller must not touch b again.
func ReleaseCacheKeyBuilder(b *CacheKeyBuilder) {
	cacheKeyBuilderPool.Put(b)
}

// Write appends a length-prefixed byte-slice field to the key.
func (b *CacheKeyBuilder) Write(p []byte) {
	binary.LittleEndian.PutUint32(b.lenBuf[:], uint32(len(p)))
	b.h.Write(b.lenBuf[:])
	b.h.Write(p)
}

// WriteString appends a length-prefixed string field to the key.
// Uses maphash's native WriteString so no copy of s is needed.
func (b *CacheKeyBuilder) WriteString(s string) {
	binary.LittleEndian.PutUint32(b.lenBuf[:], uint32(len(s)))
	b.h.Write(b.lenBuf[:])
	b.h.WriteString(s)
}

// Sum finalizes and returns the cache key. The builder can continue to
// be reused for further Write calls after Sum if desired, but typically
// the caller releases it immediately after Sum.
func (b *CacheKeyBuilder) Sum() CacheKey {
	return CacheKey(b.h.Sum64())
}

// CacheKeyOf is a shortcut for a single-field byte-slice key. It is
// equivalent to acquiring a builder, calling Write(p), Sum, and
// releasing, but avoids the pool round-trip for the common single
// field case.
func CacheKeyOf(p []byte) CacheKey {
	var h maphash.Hash
	h.SetSeed(cacheKeySeed)
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(p)))
	h.Write(lenBuf[:])
	h.Write(p)
	return CacheKey(h.Sum64())
}

// CacheKeyOfString is the string counterpart of CacheKeyOf.
func CacheKeyOfString(s string) CacheKey {
	var h maphash.Hash
	h.SetSeed(cacheKeySeed)
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(s)))
	h.Write(lenBuf[:])
	h.WriteString(s)
	return CacheKey(h.Sum64())
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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v := c.store[key]
	if v == nil {
		return nil
	}
	now := expireCacheNow()
	v.peek = now
	c.scheduleEvictLocked(now)
	return v.value
}

// Set stores the value with key.
func (c *expireCache) Set(key CacheKey, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := expireCacheNow()
	c.store[key] = &expireCacheValue{
		value: value,
		peek:  now,
	}
	c.scheduleEvictLocked(now)
}

func (c *expireCache) Del(key CacheKey) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v, ok := c.store[key]
	if !ok {
		return
	}
	delete(c.store, key)
	go c.onRelease(key, v.value)
}

func (c *expireCache) Len() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return len(c.store)
}

func (c *expireCache) OnRelease(cb func(key CacheKey, value interface{})) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cb == nil {
		c.onRelease = defaultOnRelease
		return
	}
	c.onRelease = cb
}

// scheduleEvictLocked decides whether the interval has elapsed and, if so,
// arms the next interval and launches a background eviction goroutine.
// The caller must hold c.mutex.
func (c *expireCache) scheduleEvictLocked(now int64) {
	if now < c.next {
		return
	}
	c.next = now + c.interval
	go c.evict(now)
}

// evict walks c.store under the lock, removes entries whose peek is older
// than c.expire, and fires onRelease callbacks outside the lock so that a
// slow callback cannot block other cache operations.
func (c *expireCache) evict(now int64) {
	type released struct {
		k CacheKey
		v interface{}
	}

	c.mutex.Lock()
	var toRelease []released
	for k, v := range c.store {
		if now-v.peek > c.expire {
			delete(c.store, k)
			toRelease = append(toRelease, released{k, v.value})
		}
	}
	onRelease := c.onRelease
	c.mutex.Unlock()

	for _, r := range toRelease {
		go onRelease(r.k, r.v)
	}
}
