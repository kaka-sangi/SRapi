package httputil

import (
	"testing"
	"time"
)

func TestClearanceCacheGetPutAndEvict(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	cache := NewClearanceCache(ClearanceCacheConfig{MaxEntries: 2, TTL: time.Hour, Now: clock})

	cache.Put(&ClearanceBundle{
		TargetHost: "chatgpt.com",
		ProxyURL:   "http://p1:8080",
		Cookies:    map[string]string{"cf_clearance": "abc"},
	})
	if got, ok := cache.Get("chatgpt.com", "http://p1:8080"); !ok || got.Cookies["cf_clearance"] != "abc" {
		t.Fatalf("expected hit; got=%v ok=%v", got, ok)
	}
	if _, ok := cache.Get("chatgpt.com", "http://other:8080"); ok {
		t.Fatal("expected miss for different proxy")
	}

	cache.Put(&ClearanceBundle{TargetHost: "a.com", ProxyURL: "", Cookies: map[string]string{"x": "1"}})
	cache.Put(&ClearanceBundle{TargetHost: "b.com", ProxyURL: "", Cookies: map[string]string{"x": "2"}})
	if cache.Len() != 2 {
		t.Fatalf("expected LRU bound to 2, got %d", cache.Len())
	}
}

func TestClearanceCacheExpiryDropsEntry(t *testing.T) {
	start := time.Unix(1000, 0)
	current := start
	clock := func() time.Time { return current }
	cache := NewClearanceCache(ClearanceCacheConfig{MaxEntries: 4, TTL: time.Minute, Now: clock})
	cache.Put(&ClearanceBundle{
		TargetHost: "chatgpt.com",
		Cookies:    map[string]string{"a": "1"},
	})
	current = start.Add(2 * time.Minute)
	if _, ok := cache.Get("chatgpt.com", ""); ok {
		t.Fatal("expected expired entry to be a miss")
	}
	if cache.Len() != 0 {
		t.Fatal("expected expired entry to be evicted on read")
	}
}

func TestClearanceCacheInvalidate(t *testing.T) {
	cache := NewClearanceCache(ClearanceCacheConfig{})
	cache.Put(&ClearanceBundle{TargetHost: "chatgpt.com", Cookies: map[string]string{"a": "b"}})
	cache.Invalidate("chatgpt.com", "")
	if _, ok := cache.Get("chatgpt.com", ""); ok {
		t.Fatal("expected invalidated entry to be gone")
	}
}

func TestClearanceBundleCookieHeader(t *testing.T) {
	b := &ClearanceBundle{Cookies: map[string]string{"cf_clearance": "x"}}
	got := b.CookieHeader()
	if got != "cf_clearance=x" {
		t.Fatalf("CookieHeader = %q", got)
	}
}

func TestHostFromURLNormalisation(t *testing.T) {
	cases := map[string]string{
		"https://Chatgpt.COM/conversation":  "chatgpt.com",
		"chatgpt.com":                       "chatgpt.com",
		"  https://chat.openai.com/  ":      "chat.openai.com",
		"":                                  "",
	}
	for in, want := range cases {
		if got := HostFromURL(in); got != want {
			t.Errorf("HostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeProxyURLSocksUpgrade(t *testing.T) {
	if got := normalizeProxyURL("socks://h:1"); got != "socks5h://h:1" {
		t.Fatalf("socks not upgraded: %q", got)
	}
	if got := normalizeProxyURL("socks5://h:1"); got != "socks5h://h:1" {
		t.Fatalf("socks5 not upgraded: %q", got)
	}
}
