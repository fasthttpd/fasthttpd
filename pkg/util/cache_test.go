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
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")
	if len(c.store) != 3 {
		t.Fatalf("len(c.store) is not 3\n")
	}

	// NOTE: All values may be expired.
	now = c.expire + 1
	c.next = 0

	// NOTE: Get a value and extends its expiration.
	key := "key2"
	want := "value2"
	if got := c.Get(key); got != want {
		t.Fatalf("c.Get(%q) returns %v; want %v", key, got, want)
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
		t.Fatalf("len(c.store) is not 1")
	}
}
