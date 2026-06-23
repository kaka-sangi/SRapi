package linuxdoprovider

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

const defaultHTTPTimeout = 15 * time.Second

type Provider struct {
	HTTPClient interface {
		Do(req *http.Request) (*http.Response, error)
	}
}

func New() Provider {
	return Provider{}
}

// CreateSession creates a payment order via the Linux.do Credit EasyPay-compatible
// /epay/pay/submit.php endpoint.
//
// Required config keys:
//   - gateway_url / base_url: Credit instance URL (default: https://credit.linux.do)
//   - client_id / pid:        application Client ID from the Credit console
//   - client_secret / key:    application Client Secret
//   - notify_url:             webhook callback URL (e.g. https://yourdomain/api/v1/webhooks/payments/linuxdo)
//   - return_url:             user redirect after payment
//
// Optional:
//   - exchange_rate / rate:   multiplier to convert platform currency to Credit amount (default 1.0)
//   - sign_type:              MD5 (default) — matches Credit's EasyPay compat protocol
func (p Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	gatewayURL := configString(req.Config, "gateway_url", "base_url")
	if gatewayURL == "" {
		gatewayURL = "https://credit.linux.do"
	}
	gatewayURL = strings.TrimRight(gatewayURL, "/")
	merchantID := configString(req.Config, "client_id", "pid", "merchant_id")
	signingSecret := configString(req.Config, "client_secret", "key", "signing_secret", "secret")
	notifyURL := configString(req.Config, "notify_url", "webhook_url")
	returnURL := configString(req.Config, "return_url", "success_url")
	if merchantID == "" || signingSecret == "" || notifyURL == "" || returnURL == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}

	exchangeRate := configFloat(req.Config, "exchange_rate", "rate")
	if exchangeRate <= 0 {
		exchangeRate = 1.0
	}

	amount := trimMoney(req.Amount)
	if exchangeRate != 1.0 {
		f, err := strconv.ParseFloat(amount, 64)
		if err == nil {
			amount = strconv.FormatFloat(f*exchangeRate, 'f', 2, 64)
		}
	}

	submitURL, err := url.Parse(gatewayURL + "/epay/pay/submit.php")
	if err != nil {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}

	siteName := configString(req.Config, "site_name", "sitename")
	if siteName == "" {
		siteName = "SRapi"
	}

	params := submitURL.Query()
	params.Set("pid", merchantID)
	params.Set("type", "epay")
	params.Set("out_trade_no", req.OrderNo)
	params.Set("name", siteName+" · "+req.OrderNo)
	params.Set("money", amount)
	params.Set("notify_url", notifyURL)
	if strings.Contains(returnURL, "?") {
		returnURL += "&order_no=" + req.OrderNo
	} else {
		returnURL += "?order_no=" + req.OrderNo
	}
	params.Set("return_url", returnURL)

	signType := strings.ToUpper(configString(req.Config, "sign_type"))
	if signType == "" {
		signType = "MD5"
	}
	params.Set("sign_type", signType)
	params.Set("sign", sign(params, signingSecret, signType))
	submitURL.RawQuery = params.Encode()

	return checkoutprovider.Session{
		ID:  req.OrderNo,
		URL: submitURL.String(),
		Metadata: map[string]any{
			"linuxdo_pay_url":   submitURL.String(),
			"linuxdo_method":    "linuxdo",
			"linuxdo_sign_type": signType,
			"exchange_rate":     exchangeRate,
		},
	}, nil
}

// Refund calls the EasyPay-compatible /api.php refund endpoint.
func (p Provider) Refund(req checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	apiURL := configString(req.Config, "gateway_url", "base_url", "api_url")
	if apiURL == "" {
		apiURL = "https://credit.linux.do"
	}
	apiURL = strings.TrimRight(apiURL, "/") + "/epay/api.php"
	merchantID := configString(req.Config, "client_id", "pid", "merchant_id")
	signingSecret := configString(req.Config, "client_secret", "key", "signing_secret", "secret")
	if merchantID == "" || signingSecret == "" {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}

	params := url.Values{}
	params.Set("act", "refund")
	params.Set("pid", merchantID)
	params.Set("key", signingSecret)
	params.Set("trade_no", req.ProviderTransactionID)
	params.Set("out_trade_no", req.OrderNo)
	params.Set("money", trimMoney(req.Amount))

	payload, err := p.postForm(apiURL, params)
	if err != nil {
		return checkoutprovider.RefundResult{}, err
	}

	code := mapString(payload, "code")
	if code != "1" && code != "0" {
		msg := mapString(payload, "msg")
		return checkoutprovider.RefundResult{}, fmt.Errorf("%w: %s", checkoutprovider.ErrUnavailable, msg)
	}

	return checkoutprovider.RefundResult{
		ID:       mapString(payload, "refund_id", "trade_no"),
		Status:   checkoutprovider.RefundStatusSucceeded,
		Metadata: payload,
	}, nil
}

// QueryOrder calls the EasyPay-compatible /api.php query endpoint.
func (p Provider) QueryOrder(req checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	apiURL := configString(req.Config, "gateway_url", "base_url", "api_url")
	if apiURL == "" {
		apiURL = "https://credit.linux.do"
	}
	apiURL = strings.TrimRight(apiURL, "/") + "/epay/api.php"
	merchantID := configString(req.Config, "client_id", "pid", "merchant_id")
	signingSecret := configString(req.Config, "client_secret", "key", "signing_secret", "secret")
	if merchantID == "" || signingSecret == "" {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrInvalidConfig
	}

	params := url.Values{}
	params.Set("act", "order")
	params.Set("pid", merchantID)
	params.Set("key", signingSecret)
	params.Set("out_trade_no", req.OrderNo)

	payload, err := p.postForm(apiURL, params)
	if err != nil {
		return checkoutprovider.QueryResult{}, err
	}

	status := mapString(payload, "status")
	var qs checkoutprovider.QueryStatus
	switch status {
	case "1":
		qs = checkoutprovider.QueryStatusPaid
	case "0":
		qs = checkoutprovider.QueryStatusPending
	default:
		qs = checkoutprovider.QueryStatusFailed
	}

	return checkoutprovider.QueryResult{
		Status:                qs,
		ProviderTransactionID: mapString(payload, "trade_no"),
		Amount:                mapString(payload, "money"),
		Currency:              strings.ToUpper(firstNonEmpty(mapString(payload, "currency"), req.Currency)),
		Metadata:              payload,
	}, nil
}

func (p Provider) postForm(endpoint string, params url.Values) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", checkoutprovider.ErrUnavailable, resp.StatusCode)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	return payload, nil
}

func sign(values url.Values, secret string, signType string) string {
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

func configFloat(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := values[key]; ok {
			switch typed := v.(type) {
			case float64:
				return typed
			case float32:
				return float64(typed)
			case int:
				return float64(typed)
			case string:
				f, err := strconv.ParseFloat(typed, 64)
				if err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := values[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
