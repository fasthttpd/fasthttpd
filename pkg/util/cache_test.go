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

func Test_expireCache_expires_return2nd(t *testing.T) {
	c := NewExpireCache(0).(*expireCache)
	c.expire = 0

	c.mutex.Lock()
	now := c.next + 1
	go c.expires(now)

	time.Sleep(1 * time.Millisecond)
	c.next = now + 1
	c.mutex.Unlock()

	c.mutex.Lock()
	wantNext := c.next
	c.mutex.Unlock()

	if c.next != wantNext {
		t.Errorf("unexpected c.next %d; want %d", c.next, wantNext)
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
	c.expires(now + 2)

	<-done
	c.OnRelease(nil)
}
