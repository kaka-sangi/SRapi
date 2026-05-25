package alipayprovider

import (
	"strings"
	"testing"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

func TestCreateSessionBuildsPagePayRequest(t *testing.T) {
	fake := &fakePagePayCreator{session: PagePaySession{URL: "https://openapi.alipay.test/gateway?sign=test"}}
	provider := Provider{Creator: fake}
	session, err := provider.CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":            "app_123",
			"private_key":       "app-private-key",
			"alipay_public_key": "alipay-public-key",
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
			"subject":           "SRapi top-up",
		},
		OrderNo:  "pay_123",
		Amount:   "12.34000000",
		Currency: "CNY",
		Metadata: map[string]any{"method": "alipay"},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID != "pay_123" || session.URL != fake.session.URL || session.Metadata["alipay_mode"] != "page" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if fake.last.AppID != "app_123" || fake.last.Amount != "12.34" || fake.last.Subject != "SRapi top-up" || fake.last.Mode != "page" {
		t.Fatalf("unexpected page pay request: %+v", fake.last)
	}
	if !strings.Contains(fake.last.ReturnURL, "order_no=pay_123") {
		t.Fatalf("expected return url to include order number, got %q", fake.last.ReturnURL)
	}
}

func TestCreateSessionSupportsWapMode(t *testing.T) {
	fake := &fakePagePayCreator{session: PagePaySession{URL: "https://openapi.alipay.test/wap?sign=test"}}
	provider := Provider{Creator: fake}
	_, err := provider.CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":      "app_123",
			"private_key": "app-private-key",
			"notify_url":  "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":  "https://app.example/payments/return",
			"mode":        "wap",
		},
		OrderNo: "pay_123",
		Amount:  "5.00",
	})
	if err != nil {
		t.Fatalf("create wap session: %v", err)
	}
	if fake.last.Mode != "wap" {
		t.Fatalf("expected wap mode, got %+v", fake.last)
	}
}

func TestCreateSessionRejectsInvalidConfig(t *testing.T) {
	_, err := Provider{}.CreateSession(checkoutprovider.Request{
		Config:  map[string]any{"app_id": "app_123"},
		OrderNo: "pay_123",
		Amount:  "12.34",
	})
	if err == nil {
		t.Fatal("expected invalid config error")
	}
}

func TestCreateSessionRejectsInvalidReturnURL(t *testing.T) {
	fake := &fakePagePayCreator{session: PagePaySession{URL: "https://openapi.alipay.test/gateway?sign=test"}}
	_, err := (Provider{Creator: fake}).CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":      "app_123",
			"private_key": "app-private-key",
			"notify_url":  "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":  "not-a-url",
		},
		OrderNo: "pay_123",
		Amount:  "12.34",
	})
	if err == nil {
		t.Fatal("expected invalid return URL config error")
	}
	if fake.last.AppID != "" {
		t.Fatalf("checkout provider should reject before calling SDK, got %+v", fake.last)
	}
}

type fakePagePayCreator struct {
	last    PagePayRequest
	session PagePaySession
	err     error
}

func (f *fakePagePayCreator) CreatePagePay(req PagePayRequest) (PagePaySession, error) {
	f.last = req
	if f.err != nil {
		return PagePaySession{}, f.err
	}
	return f.session, nil
}
