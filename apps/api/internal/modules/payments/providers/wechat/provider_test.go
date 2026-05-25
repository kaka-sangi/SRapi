package wechatprovider

import (
	"errors"
	"testing"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

func TestCreateSessionBuildsNativePrepayRequest(t *testing.T) {
	fake := &fakePrepayCreator{session: PrepaySession{
		ID:  "pay_123",
		URL: "weixin://wxpay/bizpayurl?pr=test",
		Metadata: map[string]any{
			"custom": "ok",
		},
	}}
	session, err := (Provider{Creator: fake}).CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":          "wx_app_123",
			"mch_id":          "mch_123",
			"api_v3_key":      "0123456789abcdef0123456789abcdef",
			"serial_no":       "serial_123",
			"private_key":     "private-key",
			"notify_url":      "https://api.example/api/v1/webhooks/payments/wechat",
			"payer_client_ip": "127.0.0.1",
		},
		OrderNo:  "pay_123",
		Amount:   "12.34000000",
		Currency: "CNY",
		Metadata: map[string]any{"method": "wechat"},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID != "pay_123" || session.URL != fake.session.URL || session.Metadata["wechat_code_url"] != nil {
		t.Fatalf("unexpected checkout session: %+v", session)
	}
	if session.Metadata["wechat_pay_mode"] != "native" || session.Metadata["wechat_pay_url"] != fake.session.URL || session.Metadata["custom"] != "ok" {
		t.Fatalf("expected native metadata, got %+v", session.Metadata)
	}
	if fake.last.AppID != "wx_app_123" || fake.last.MchID != "mch_123" || fake.last.Amount != 1234 || fake.last.Mode != "native" {
		t.Fatalf("unexpected prepay request: %+v", fake.last)
	}
	if fake.last.Description != "SRapi order pay_123" || fake.last.NotifyURL != "https://api.example/api/v1/webhooks/payments/wechat" {
		t.Fatalf("unexpected prepay description or notify URL: %+v", fake.last)
	}
}

func TestCreateSessionBuildsJSAPIPrepayRequest(t *testing.T) {
	fake := &fakePrepayCreator{session: PrepaySession{ID: "prepay_id=test"}}
	_, err := (Provider{Creator: fake}).CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":      "wx_app_123",
			"mch_id":      "mch_123",
			"api_v3_key":  "0123456789abcdef0123456789abcdef",
			"serial_no":   "serial_123",
			"private_key": "private-key",
			"notify_url":  "https://api.example/api/v1/webhooks/payments/wechat",
			"mode":        "jsapi",
		},
		OrderNo:  "pay_123",
		Amount:   "12.34",
		Currency: "CNY",
		Metadata: map[string]any{"payer_openid": "openid_123"},
	})
	if err != nil {
		t.Fatalf("create jsapi session: %v", err)
	}
	if fake.last.Mode != "jsapi" || fake.last.PayerOpenID != "openid_123" {
		t.Fatalf("unexpected jsapi request: %+v", fake.last)
	}
}

func TestCreateSessionRejectsInvalidWechatConfig(t *testing.T) {
	_, err := Provider{Creator: &fakePrepayCreator{}}.CreateSession(checkoutprovider.Request{
		Config:  map[string]any{"app_id": "wx_app_123"},
		OrderNo: "pay_123",
		Amount:  "12.34",
	})
	if !errors.Is(err, checkoutprovider.ErrInvalidConfig) {
		t.Fatalf("expected invalid config, got %v", err)
	}
}

func TestCreateSessionWrapsWechatPrepayFailure(t *testing.T) {
	_, err := (Provider{Creator: &fakePrepayCreator{err: errors.New("wechat unavailable")}}).CreateSession(checkoutprovider.Request{
		Config: map[string]any{
			"app_id":      "wx_app_123",
			"mch_id":      "mch_123",
			"api_v3_key":  "0123456789abcdef0123456789abcdef",
			"serial_no":   "serial_123",
			"private_key": "private-key",
			"notify_url":  "https://api.example/api/v1/webhooks/payments/wechat",
		},
		OrderNo:  "pay_123",
		Amount:   "12.34",
		Currency: "CNY",
	})
	if !errors.Is(err, checkoutprovider.ErrUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

type fakePrepayCreator struct {
	last    PrepayRequest
	session PrepaySession
	err     error
}

func (f *fakePrepayCreator) CreatePrepay(req PrepayRequest) (PrepaySession, error) {
	f.last = req
	if f.err != nil {
		return PrepaySession{}, f.err
	}
	return f.session, nil
}
