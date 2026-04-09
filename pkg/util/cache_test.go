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

func Test_expireCache(t *testing.T) {
	expireCacheNowOrg := expireCacheNow
	defer func() { expireCacheNow = expireCacheNowOrg }()

	now := int64(0)
	expireCacheNow = func() int64 {
		return now
	}

	c := NewExpireCache(0).(*expireCache)
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

func Test_expireCache_OnRelease(t *testing.T) {
	expireCacheNowOrg := expireCacheNow
	defer func() { expireCacheNow = expireCacheNowOrg }()

	now := int64(0)
	expireCacheNow = func() int64 {
		return now
	}

	done := make(chan bool)
	key := CacheKeyOfString("key")
	value := "value"

	c := NewExpireCacheInterval(1, 1).(*expireCache)
	c.OnRelease(func(k CacheKey, v interface{}) {
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
