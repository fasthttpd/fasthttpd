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
	Get(key CacheKey) any
	Set(key CacheKey, value any)
	Del(key CacheKey)
	Len() int
	// OnRelease sets a callback that will be called on the key is released.
	OnRelease(cb func(key CacheKey, value any))
}

const (
	// defaultCacheExpire is 5 * 60 * 1000 ms (5 min)
	defaultCacheExpire = 5 * 60 * 1000
	// defaultCacheInterval is 60 * 1000 ms (1 min)
	defaultCacheInterval = 60 * 1000
	// defaultCacheStoreSize is the initial map capacity used when the
	// caller does not specify MaxEntries.
	defaultCacheStoreSize = 1024
)

// cacheNow returns the current time in milliseconds since the Unix
// epoch. Tests swap it to inject a deterministic clock.
var cacheNow = func() int64 {
	return time.Now().UnixMilli()
}

type cacheValue struct {
	value any
	peek  int64
}

type cache struct {
	store     map[CacheKey]*cacheValue
	expire    int64
	interval  int64
	next      int64
	max       int
	mutex     sync.Mutex
	onRelease func(key CacheKey, value any)
}

var _ Cache = (*cache)(nil)

// CacheConfig configures a Cache produced by NewCache.
//
// All durations are in milliseconds. Zero (or negative) values for
// Expire and Interval fall back to package defaults; a non-positive
// MaxEntries means the cache is unbounded.
type CacheConfig struct {
	// Expire is the entry TTL in milliseconds. An entry whose peek
	// timestamp is older than Expire is removed on the next eviction
	// pass.
	Expire int64
	// Interval is the minimum gap, in milliseconds, between background
	// eviction passes.
	Interval int64
	// MaxEntries caps the number of stored entries. When the cap is
	// reached, Set on a new key is dropped (existing entries are
	// preserved); this prioritizes already-cached hot paths over
	// adversarial unique-key floods. Zero or negative means unbounded.
	MaxEntries int
}

// NewCache returns a new Cache configured by cfg.
func NewCache(cfg CacheConfig) Cache {
	storeSize := cfg.MaxEntries
	if storeSize <= 0 {
		storeSize = defaultCacheStoreSize
	}
	c := &cache{
		store:    make(map[CacheKey]*cacheValue, storeSize),
		expire:   cfg.Expire,
		interval: cfg.Interval,
		max:      cfg.MaxEntries,
	}
	if c.expire <= 0 {
		c.expire = defaultCacheExpire
	}
	if c.interval <= 0 {
		c.interval = defaultCacheInterval
	}
	c.next = cacheNow() + c.interval
	return c
}

// Get returns the value mapped to the specified key and extends its expiration.
func (c *cache) Get(key CacheKey) any {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v := c.store[key]
	if v == nil {
		return nil
	}
	now := cacheNow()
	v.peek = now
	c.scheduleEvictLocked(now)
	return v.value
}

// Set stores the value with key. When a maxEntries cap is configured
// and the cache already holds that many entries, Set on a new key is
// dropped; existing keys can still be updated in place.
func (c *cache) Set(key CacheKey, value any) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := cacheNow()
	if c.max > 0 && len(c.store) >= c.max {
		if _, exists := c.store[key]; !exists {
			c.scheduleEvictLocked(now)
			return
		}
	}
	c.store[key] = &cacheValue{
		value: value,
		peek:  now,
	}
	c.scheduleEvictLocked(now)
}

func (c *cache) Del(key CacheKey) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v, ok := c.store[key]
	if !ok {
		return
	}
	delete(c.store, key)
	if c.onRelease != nil {
		go c.onRelease(key, v.value)
	}
}

func (c *cache) Len() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return len(c.store)
}

// OnRelease registers a callback invoked when a key is removed either
// by Del or by the background eviction pass. Passing nil clears the
// callback; while no callback is set, Del and eviction skip goroutine
// dispatch entirely and leave the hot path free of scheduler churn.
func (c *cache) OnRelease(cb func(key CacheKey, value any)) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.onRelease = cb
}

// scheduleEvictLocked decides whether the interval has elapsed and, if so,
// arms the next interval and launches a background eviction goroutine.
// The caller must hold c.mutex.
func (c *cache) scheduleEvictLocked(now int64) {
	if now < c.next {
		return
	}
	c.next = now + c.interval
	go c.evict(now)
}

// evict walks c.store under the lock, removes entries whose peek is older
// than c.expire, and fires onRelease callbacks outside the lock so that a
// slow callback cannot block other cache operations.
//
// When no onRelease callback is registered the eviction pass skips
// allocating the release list and dispatching goroutines, keeping the
// default routesCache path free of per-tick scheduler churn.
func (c *cache) evict(now int64) {
	type released struct {
		k CacheKey
		v any
	}

	c.mutex.Lock()
	onRelease := c.onRelease
	var toRelease []released
	for k, v := range c.store {
		if now-v.peek > c.expire {
			delete(c.store, k)
			if onRelease != nil {
				toRelease = append(toRelease, released{k, v.value})
			}
		}
	}
	c.mutex.Unlock()

	for _, r := range toRelease {
		go onRelease(r.k, r.v)
	}
}
