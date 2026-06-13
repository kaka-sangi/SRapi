package linuxdoprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

const (
	defaultBaseURL = "https://connect.linux.do"
	httpTimeout    = 15 * time.Second
)

type Provider struct {
	HTTPClient interface{ Do(req *http.Request) (*http.Response, error) }
}

func New() Provider {
	return Provider{}
}

func (p Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	baseURL := configString(req.Config, "base_url", "gateway_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	clientID := configString(req.Config, "client_id")
	clientSecret := configString(req.Config, "client_secret", "secret")
	notifyURL := configString(req.Config, "notify_url", "webhook_url", "callback_url")
	returnURL := configString(req.Config, "return_url", "success_url")
	if clientID == "" || clientSecret == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}

	exchangeRate := configFloat(req.Config, "exchange_rate", "rate")
	if exchangeRate <= 0 {
		exchangeRate = 1.0
	}

	amountCents, err := parseCreditAmount(req.Amount)
	if err != nil {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}

	creditAmount := int(float64(amountCents) * exchangeRate)

	body := map[string]any{
		"order_no":    req.OrderNo,
		"amount":      creditAmount,
		"description": "SRapi credit purchase · " + req.OrderNo,
	}
	if notifyURL != "" {
		body["notify_url"] = notifyURL
	}
	if returnURL != "" {
		body["return_url"] = appendOrderNo(returnURL, req.OrderNo)
	}
	if req.UserID > 0 {
		body["user_id"] = req.UserID
	}

	payload, err := p.callAPI(baseURL+"/api/v1/payments/create", clientID, clientSecret, body)
	if err != nil {
		return checkoutprovider.Session{}, err
	}

	payURL := mapString(payload, "payment_url", "pay_url", "url", "redirect_url")
	sessionID := mapString(payload, "id", "payment_id", "transaction_id")
	if sessionID == "" {
		sessionID = req.OrderNo
	}

	return checkoutprovider.Session{
		ID:  sessionID,
		URL: payURL,
		Metadata: map[string]any{
			"linuxdo_payment_url": payURL,
			"linuxdo_session_id":  sessionID,
		},
	}, nil
}

func (p Provider) Refund(req checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	baseURL := configString(req.Config, "base_url", "gateway_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	clientID := configString(req.Config, "client_id")
	clientSecret := configString(req.Config, "client_secret", "secret")
	if clientID == "" || clientSecret == "" {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}

	amountCents, _ := parseCreditAmount(req.Amount)
	body := map[string]any{
		"order_no":       req.OrderNo,
		"amount":         amountCents,
		"reason":         req.Reason,
		"idempotency_key": req.IdempotencyKey,
	}

	payload, err := p.callAPI(baseURL+"/api/v1/payments/refund", clientID, clientSecret, body)
	if err != nil {
		return checkoutprovider.RefundResult{}, err
	}

	return checkoutprovider.RefundResult{
		ID:       mapString(payload, "refund_id", "id"),
		Status:   checkoutprovider.RefundStatusSucceeded,
		Metadata: payload,
	}, nil
}

func (p Provider) QueryOrder(req checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	baseURL := configString(req.Config, "base_url", "gateway_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	clientID := configString(req.Config, "client_id")
	clientSecret := configString(req.Config, "client_secret", "secret")
	if clientID == "" || clientSecret == "" {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrInvalidConfig
	}

	body := map[string]any{"order_no": req.OrderNo}
	payload, err := p.callAPI(baseURL+"/api/v1/payments/query", clientID, clientSecret, body)
	if err != nil {
		return checkoutprovider.QueryResult{}, err
	}

	status := strings.ToLower(mapString(payload, "status", "payment_status"))
	var qs checkoutprovider.QueryStatus
	switch status {
	case "paid", "success", "succeeded", "completed":
		qs = checkoutprovider.QueryStatusPaid
	case "failed", "fail":
		qs = checkoutprovider.QueryStatusFailed
	case "canceled", "cancelled", "closed":
		qs = checkoutprovider.QueryStatusCanceled
	default:
		qs = checkoutprovider.QueryStatusPending
	}

	return checkoutprovider.QueryResult{
		Status:                qs,
		ProviderTransactionID: mapString(payload, "transaction_id", "id"),
		Amount:                mapString(payload, "amount"),
		Currency:              strings.ToUpper(firstNonEmpty(mapString(payload, "currency"), req.Currency)),
		Metadata:              payload,
	}, nil
}

func (p Provider) callAPI(endpoint, clientID, clientSecret string, body map[string]any) (map[string]any, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+clientSecret)
	req.Header.Set("X-Client-ID", clientID)

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %s", checkoutprovider.ErrUnavailable, string(data))
	}

	payload := map[string]any{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, checkoutprovider.ErrUnavailable
	}
	return payload, nil
}

func parseCreditAmount(amount string) (int, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0, fmt.Errorf("empty amount")
	}
	if strings.Contains(amount, ".") {
		f, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			return 0, err
		}
		return int(f * 100), nil
	}
	n, err := strconv.Atoi(amount)
	if err != nil {
		return 0, err
	}
	return n * 100, nil
}

func appendOrderNo(rawURL string, orderNo string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&order_no=" + orderNo
	}
	return rawURL + "?order_no=" + orderNo
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
