package util

import (
	"testing"
	"time"
)

func TestCacheKey(t *testing.T) {
	b := CacheKeyBytes([]byte("GET"), []byte{' '}, []byte("/path"))
	s := CacheKeyString("GET", " ", "/path")
	if b != s {
		t.Errorf("different results CacheKeyBytes %d, CacheKeyString %d", b, s)
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
	c.Set(CacheKeyString("key1"), "value1")
	c.Set(CacheKeyString("key2"), "value2")
	c.Set(CacheKeyString("key3"), "value3")
	if c.Len() != 3 {
		t.Fatalf("unexpected size %d\n", c.Len())
	}

	// NOTE: All values may be expired.
	now = c.expire + 1
	c.mutex.Lock()
	c.next = 0
	c.mutex.Unlock()

	// NOTE: Get a value and extends its expiration.
	if got := c.Get(CacheKeyString("key2")); got != "value2" {
		t.Fatalf("unexpected value %v; want %v", got, "value2")
	}

	if got := c.Get(CacheKeyString("unknown")); got != nil {
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
	key := CacheKeyString("key")
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
