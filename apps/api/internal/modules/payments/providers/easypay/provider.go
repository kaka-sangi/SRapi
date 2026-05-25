package easypayprovider

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

type Provider struct{}

func New() Provider {
	return Provider{}
}

func (Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	gatewayURL := configString(req.Config, "gateway_url", "base_url", "payment_url")
	if gatewayURL == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	merchantID := configString(req.Config, "merchant_id", "pid")
	signingSecret := configString(req.Config, "signing_secret", "webhook_secret", "key", "secret")
	notifyURL := configString(req.Config, "notify_url", "webhook_url")
	returnURL := configString(req.Config, "return_url", "success_url")
	if merchantID == "" || signingSecret == "" || notifyURL == "" || returnURL == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	parsed, err := url.Parse(gatewayURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	params := parsed.Query()
	params.Set("pid", merchantID)
	params.Set("type", easypayMethod(req.Metadata))
	params.Set("out_trade_no", req.OrderNo)
	params.Set("name", "SRapi order "+req.OrderNo)
	params.Set("money", trimMoney(req.Amount))
	params.Set("notify_url", notifyURL)
	returnURL = appendOrderNo(returnURL, req.OrderNo)
	if returnURL == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	params.Set("return_url", returnURL)
	params.Set("sitename", configString(req.Config, "site_name", "sitename"))
	params.Set("currency", strings.ToUpper(strings.TrimSpace(req.Currency)))
	signType := strings.ToUpper(configString(req.Config, "sign_type"))
	if signType == "" {
		signType = "MD5"
	}
	params.Set("sign_type", signType)
	params.Set("sign", signEasypay(params, signingSecret, signType))
	parsed.RawQuery = params.Encode()
	return checkoutprovider.Session{
		ID:  req.OrderNo,
		URL: parsed.String(),
		Metadata: map[string]any{
			"easypay_pay_url":   parsed.String(),
			"easypay_method":    params.Get("type"),
			"easypay_sign":      params.Get("sign"),
			"easypay_sign_type": signType,
		},
	}, nil
}

func signEasypay(values url.Values, secret string, signType string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "sign" || key == "sign_type" {
			continue
		}
		if strings.TrimSpace(values.Get(key)) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	canonical := strings.Join(parts, "&")
	if signType == "HMAC-SHA256" || signType == "SHA256" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(canonical))
		return hex.EncodeToString(mac.Sum(nil))
	}
	sum := md5.Sum([]byte(canonical + secret))
	return hex.EncodeToString(sum[:])
}

func easypayMethod(metadata map[string]any) string {
	method := strings.ToLower(strings.TrimSpace(metadataString(metadata, "method", "payment_method")))
	switch method {
	case "alipay", "wxpay", "wechat", "qqpay", "bank":
		if method == "wechat" {
			return "wxpay"
		}
		return method
	default:
		return "alipay"
	}
}

func appendOrderNo(rawURL string, orderNo string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	query := parsed.Query()
	if query.Get("order_no") == "" {
		query.Set("order_no", orderNo)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func trimMoney(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ".") {
		value = strings.TrimRight(value, "0")
		value = strings.TrimRight(value, ".")
	}
	if value == "" {
		return "0"
	}
	return value
}

func configString(values map[string]any, keys ...string) string {
	return mapString(values, keys...)
}

func metadataString(values map[string]any, keys ...string) string {
	return mapString(values, keys...)
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case fmt.Stringer:
			return strings.TrimSpace(typed.String())
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
}
