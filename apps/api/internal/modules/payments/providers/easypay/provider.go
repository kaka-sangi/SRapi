package easypayprovider

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

const defaultEasyPayTimeout = 15 * time.Second

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Provider struct {
	HTTPClient HTTPDoer
}

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

func (p Provider) Refund(req checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	apiURL := configString(req.Config, "refund_url", "api_url", "gateway_url", "base_url")
	merchantID := configString(req.Config, "merchant_id", "pid")
	signingSecret := configString(req.Config, "signing_secret", "webhook_secret", "key", "secret")
	if apiURL == "" || merchantID == "" || signingSecret == "" || strings.TrimSpace(req.OrderNo) == "" {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}
	params, endpoint, err := easypayAPIParams(apiURL)
	if err != nil {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}
	params.Set("act", "refund")
	params.Set("pid", merchantID)
	params.Set("out_trade_no", req.OrderNo)
	params.Set("money", trimMoney(req.Amount))
	params.Set("refund_no", req.IdempotencyKey)
	params.Set("reason", strings.TrimSpace(req.Reason))
	signType := strings.ToUpper(configString(req.Config, "sign_type"))
	if signType == "" {
		signType = "MD5"
	}
	params.Set("sign_type", signType)
	params.Set("sign", signEasypay(params, signingSecret, signType))
	payload, err := p.callAPI(endpoint, params)
	if err != nil {
		return checkoutprovider.RefundResult{}, err
	}
	if !easypayOK(payload) {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrUnavailable
	}
	status := checkoutprovider.RefundStatusSucceeded
	switch strings.ToLower(mapString(payload, "status", "refund_status")) {
	case "processing", "pending", "refunding":
		status = checkoutprovider.RefundStatusProcessing
	case "failed", "fail", "refund_failed":
		status = checkoutprovider.RefundStatusFailed
	}
	return checkoutprovider.RefundResult{
		ID:       firstNonEmpty(mapString(payload, "refund_id", "refund_no"), req.IdempotencyKey),
		Status:   status,
		Metadata: payload,
	}, nil
}

func (p Provider) QueryOrder(req checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	apiURL := configString(req.Config, "query_url", "api_url", "gateway_url", "base_url")
	merchantID := configString(req.Config, "merchant_id", "pid")
	signingSecret := configString(req.Config, "signing_secret", "webhook_secret", "key", "secret")
	if apiURL == "" || merchantID == "" || signingSecret == "" || strings.TrimSpace(req.OrderNo) == "" {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrInvalidConfig
	}
	params, endpoint, err := easypayAPIParams(apiURL)
	if err != nil {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrInvalidConfig
	}
	params.Set("act", "order")
	params.Set("pid", merchantID)
	params.Set("out_trade_no", req.OrderNo)
	signType := strings.ToUpper(configString(req.Config, "sign_type"))
	if signType == "" {
		signType = "MD5"
	}
	params.Set("sign_type", signType)
	params.Set("sign", signEasypay(params, signingSecret, signType))
	payload, err := p.callAPI(endpoint, params)
	if err != nil {
		return checkoutprovider.QueryResult{}, err
	}
	if !easypayOK(payload) {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrUnavailable
	}
	return checkoutprovider.QueryResult{
		Status:                easypayQueryStatus(mapString(payload, "status", "trade_status")),
		ProviderTransactionID: mapString(payload, "trade_no", "transaction_id", "provider_transaction_id"),
		Amount:                normalizeEasyPayAmount(mapString(payload, "money", "amount")),
		Currency:              strings.ToUpper(firstNonEmpty(mapString(payload, "currency"), req.Currency)),
		Metadata:              payload,
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

func (p Provider) callAPI(endpoint string, params url.Values) (map[string]any, error) {
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultEasyPayTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, checkoutprovider.ErrUnavailable
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return nil, checkoutprovider.ErrUnavailable
		}
		for key := range values {
			payload[key] = values.Get(key)
		}
	}
	return payload, nil
}

func easypayAPIParams(rawURL string) (url.Values, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, "", checkoutprovider.ErrInvalidConfig
	}
	params := parsed.Query()
	parsed.RawQuery = ""
	return params, parsed.String(), nil
}

func easypayOK(payload map[string]any) bool {
	code := strings.ToLower(mapString(payload, "code", "status_code"))
	if code == "0" || code == "1" || code == "success" || code == "ok" {
		return true
	}
	status := strings.ToLower(mapString(payload, "status", "trade_status"))
	return status == "paid" || status == "success" || status == "succeeded" || status == "processing"
}

func easypayQueryStatus(status string) checkoutprovider.QueryStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "1":
		return checkoutprovider.QueryStatusPaid
	case "failed", "fail":
		return checkoutprovider.QueryStatusFailed
	case "closed", "cancelled", "canceled":
		return checkoutprovider.QueryStatusCanceled
	default:
		return checkoutprovider.QueryStatusPending
	}
}

func normalizeEasyPayAmount(value string) string {
	value = trimMoney(value)
	if !strings.Contains(value, ".") {
		return value + ".00000000"
	}
	parts := strings.SplitN(value, ".", 2)
	fraction := parts[1]
	for len(fraction) < 8 {
		fraction += "0"
	}
	if len(fraction) > 8 {
		fraction = fraction[:8]
	}
	return parts[0] + "." + fraction
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
