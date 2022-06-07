package util

import (
	"testing"
	"time"
)

func Test_expireCache_Typical(t *testing.T) {
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
	if len(c.store) != 3 {
		t.Fatalf("unexpected size %d\n", len(c.store))
	}

	// NOTE: All values may be expired.
	now = c.expire + 1
	c.next = 0

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
		if len(c.store) == 1 {
			ok = true
			break
		}
		if tries < 0 {
			break
		}
	}
	if !ok {
		t.Fatalf("unexpected size %d\n", len(c.store))
	}
}

func Test_expireCache_notify_return2nd(t *testing.T) {
	c := NewExpireCache(0).(*expireCache)

	c.mutex.Lock()

	now := c.next + 1
	c.notify(now)
	c.next = now + 1
	wantNext := c.next

	c.mutex.Unlock()

	if c.next != wantNext {
		t.Errorf("unexpected c.next %d; want %d", c.next, wantNext)
	}
}
