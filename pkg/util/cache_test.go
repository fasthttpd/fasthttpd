package util

import (
	"testing"
	"time"
)

func TestCacheKey(t *testing.T) {
	// Write and WriteString must produce the same key for the same
	// field contents, so a caller can freely pick whichever type is
	// more convenient without breaking cache lookups.
	kbBytes := AcquireCacheKeyBuilder()
	kbBytes.Write([]byte("GET"))
	kbBytes.Write([]byte(" "))
	kbBytes.Write([]byte("/path"))
	byKey := kbBytes.Sum()
	ReleaseCacheKeyBuilder(kbBytes)

	kbStr := AcquireCacheKeyBuilder()
	kbStr.WriteString("GET")
	kbStr.WriteString(" ")
	kbStr.WriteString("/path")
	strKey := kbStr.Sum()
	ReleaseCacheKeyBuilder(kbStr)

	if byKey != strKey {
		t.Errorf("Write and WriteString disagree: %d vs %d", byKey, strKey)
	}
}

// TestCacheKey_FieldSeparator guards against the classic hash-composition
// bug where concatenating fields without a delimiter lets ("ab", "c") and
// ("a", "bc") collide. The length-prefix scheme inside CacheKeyBuilder
// makes these inputs produce distinct keys.
func TestCacheKey_FieldSeparator(t *testing.T) {
	build := func(fields ...string) CacheKey {
		kb := AcquireCacheKeyBuilder()
		for _, f := range fields {
			kb.WriteString(f)
		}
		key := kb.Sum()
		ReleaseCacheKeyBuilder(kb)
		return key
	}

	testCases := []struct {
		caseName string
		a, b     []string
	}{
		{
			caseName: "two-field split",
			a:        []string{"ab", "c"},
			b:        []string{"a", "bc"},
		},
		{
			caseName: "three-field split",
			a:        []string{"GET", "/api", "/users"},
			b:        []string{"GE", "T/api/", "users"},
		},
		{
			caseName: "empty-field sensitivity",
			a:        []string{"", "foo"},
			b:        []string{"foo"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			if got := build(tc.a...); got == build(tc.b...) {
				t.Errorf("field split %v and %v collide on key %d", tc.a, tc.b, got)
			}
		})
	}
}

// TestCacheKey_ShortcutMatchesBuilder verifies that the single-field
// shortcut helpers produce the same value as calling the builder with
// one field — otherwise callers that mix them in the same cache would
// silently see misses.
func TestCacheKey_ShortcutMatchesBuilder(t *testing.T) {
	const s = "192.168.1.1:54321"

	kb := AcquireCacheKeyBuilder()
	kb.WriteString(s)
	builderKey := kb.Sum()
	ReleaseCacheKeyBuilder(kb)

	if got := CacheKeyOfString(s); got != builderKey {
		t.Errorf("CacheKeyOfString = %d, builder = %d", got, builderKey)
	}
	if got := CacheKeyOf([]byte(s)); got != builderKey {
		t.Errorf("CacheKeyOf = %d, builder = %d", got, builderKey)
	}
}

func TestCache(t *testing.T) {
	cacheNowOrg := cacheNow
	defer func() { cacheNow = cacheNowOrg }()

	now := int64(0)
	cacheNow = func() int64 {
		return now
	}

	c := NewCache(CacheConfig{}).(*cache)
	c.Set(CacheKeyOfString("key1"), "value1")
	c.Set(CacheKeyOfString("key2"), "value2")
	c.Set(CacheKeyOfString("key3"), "value3")
	if c.Len() != 3 {
		t.Fatalf("unexpected size %d\n", c.Len())
	}

	// NOTE: All values may be expired.
	now = c.expire + 1
	c.mutex.Lock()
	c.next = 0
	c.mutex.Unlock()

	// NOTE: Get a value and extends its expiration.
	if got := c.Get(CacheKeyOfString("key2")); got != "value2" {
		t.Fatalf("unexpected value %v; want %v", got, "value2")
	}

	if got := c.Get(CacheKeyOfString("unknown")); got != nil {
		t.Fatalf("unexpected value %v", got)
	}

	tries := 5
	ok := false
	for {
		time.Sleep(1 * time.Millisecond)
		tries--
		if c.Len() == 1 {
			ok = true
			break
		}
		if tries < 0 {
			break
		}
	}
	if !ok {
		t.Fatalf("unexpected size %d\n", c.Len())
	}
}

func TestCache_OnRelease(t *testing.T) {
	cacheNowOrg := cacheNow
	defer func() { cacheNow = cacheNowOrg }()

	now := int64(0)
	cacheNow = func() int64 {
		return now
	}

	done := make(chan bool)
	key := CacheKeyOfString("key")
	value := "value"

	c := NewCache(CacheConfig{Expire: 1, Interval: 1}).(*cache)
	c.OnRelease(func(k CacheKey, v any) {
		if k != key || v != value {
			t.Errorf("unexpected key value: %v %v", k, v)
		}
		done <- true
	})
	c.Set(key, value)

	// Pretend enough time has passed and trigger eviction directly.
	// scheduleEvictLocked requires the caller to hold c.mutex.
	c.mutex.Lock()
	c.scheduleEvictLocked(now + 2)
	c.mutex.Unlock()

	<-done
	c.OnRelease(nil)
}

// TestCache_NoCallback ensures Del and evict remain functional
// (key removed, no panic) when OnRelease has never been set. This
// guards the fast path that skips goroutine dispatch in that case.
func TestCache_NoCallback(t *testing.T) {
	cacheNowOrg := cacheNow
	defer func() { cacheNow = cacheNowOrg }()

	now := int64(0)
	cacheNow = func() int64 { return now }

	c := NewCache(CacheConfig{Expire: 1, Interval: 1}).(*cache)
	keyA := CacheKeyOfString("a")
	keyB := CacheKeyOfString("b")
	c.Set(keyA, "va")
	c.Set(keyB, "vb")

	c.Del(keyA)
	if c.Len() != 1 {
		t.Fatalf("unexpected size after Del: %d", c.Len())
	}

	// Force eviction of the remaining entry.
	c.mutex.Lock()
	c.scheduleEvictLocked(now + 2)
	c.mutex.Unlock()

	// evict runs in a goroutine; spin until store drains or time out.
	for range 100 {
		if c.Len() == 0 {
			return
		}
		time.Sleep(1 * time.Millisecond)
	}
	t.Fatalf("evict did not drain store: size=%d", c.Len())
}

// TestCache_MaxEntries verifies that the drop-new policy rejects
// Set on a novel key when the cache is full, but still lets existing
// keys be updated in place and frees room once entries are removed.
func TestCache_MaxEntries(t *testing.T) {
	cacheNowOrg := cacheNow
	defer func() { cacheNow = cacheNowOrg }()

	now := int64(0)
	cacheNow = func() int64 { return now }

	c := NewCache(CacheConfig{Expire: 1000, Interval: 1000, MaxEntries: 2}).(*cache)
	keyA := CacheKeyOfString("a")
	keyB := CacheKeyOfString("b")
	keyC := CacheKeyOfString("c")

	c.Set(keyA, "va")
	c.Set(keyB, "vb")
	if c.Len() != 2 {
		t.Fatalf("unexpected size after filling: %d", c.Len())
	}

	// New key beyond the cap is dropped.
	c.Set(keyC, "vc")
	if c.Len() != 2 {
		t.Fatalf("cap violated: size=%d", c.Len())
	}
	if got := c.Get(keyC); got != nil {
		t.Fatalf("dropped key unexpectedly cached: %v", got)
	}

	// Existing key can still be updated in place.
	c.Set(keyA, "va2")
	if got := c.Get(keyA); got != "va2" {
		t.Fatalf("in-place update failed: %v", got)
	}
	if c.Len() != 2 {
		t.Fatalf("in-place update grew cache: size=%d", c.Len())
	}

	// After Del the cap frees up and a new key fits.
	c.Del(keyA)
	c.Set(keyC, "vc")
	if got := c.Get(keyC); got != "vc" {
		t.Fatalf("unexpected value after freeing cap: %v", got)
	}
}

// BenchmarkCache_Get measures the steady-state cache-hit path,
// which is the only branch reached during normal routesCache operation
// after warmup. Must remain zero-alloc.
func BenchmarkCache_Get(b *testing.B) {
	c := NewCache(CacheConfig{Expire: 60_000})
	key := CacheKeyOfString("hit")
	c.Set(key, "value")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = c.Get(key)
	}
}

// BenchmarkCache_Del_NoCallback guards the fast path that skips
// goroutine dispatch in Del when OnRelease is unset.
func BenchmarkCache_Del_NoCallback(b *testing.B) {
	c := NewCache(CacheConfig{Expire: 60_000})
	key := CacheKeyOfString("k")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Set(key, "v")
		c.Del(key)
	}
}

// BenchmarkCache_Evict_NoCallback forces an eviction pass over
// expired entries with no callback registered. The fast path skips
// allocating the release list and dispatching goroutines.
func BenchmarkCache_Evict_NoCallback(b *testing.B) {
	cacheNowOrg := cacheNow
	defer func() { cacheNow = cacheNowOrg }()

	now := int64(0)
	cacheNow = func() int64 { return now }

	c := NewCache(CacheConfig{Expire: 1, Interval: 1_000_000}).(*cache)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		// Populate a handful of entries, then force eviction at a
		// timestamp past their expiry. evict takes the lock itself.
		c.Set(CacheKeyOfString("a"), "va")
		c.Set(CacheKeyOfString("b"), "vb")
		c.Set(CacheKeyOfString("c"), "vc")
		now += 10
		c.evict(now)
	}
}

// BenchmarkCache_Set_MaxReached exercises the drop-new branch of
// Set when the cap is saturated. It is expected to allocate nothing
// because no wrapper is created.
func BenchmarkCache_Set_MaxReached(b *testing.B) {
	c := NewCache(CacheConfig{Expire: 60_000, Interval: 60_000, MaxEntries: 2})
	c.Set(CacheKeyOfString("a"), "va")
	c.Set(CacheKeyOfString("b"), "vb")
	reject := CacheKeyOfString("c")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Set(reject, "vc")
	}
}
