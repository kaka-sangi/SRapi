package alipayprovider

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"github.com/smartwalle/alipay/v3"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
)

type PagePayRequest struct {
	AppID           string
	PrivateKey      string
	AlipayPublicKey string
	Production      bool
	GatewayURL      string
	Mode            string
	OrderNo         string
	Amount          string
	Subject         string
	Body            string
	NotifyURL       string
	ReturnURL       string
	QRPayMode       string
	QRCodeWidth     string
}

type PagePaySession struct {
	URL string
}

type PagePayCreator interface {
	CreatePagePay(req PagePayRequest) (PagePaySession, error)
}

type Provider struct {
	Creator PagePayCreator
}

func New() Provider {
	return Provider{}
}

func (p Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	payReq, err := pagePayRequest(req)
	if err != nil {
		return checkoutprovider.Session{}, err
	}
	session, err := p.createPagePay(payReq)
	if err != nil {
		return checkoutprovider.Session{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	if strings.TrimSpace(session.URL) == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	return checkoutprovider.Session{
		ID:  req.OrderNo,
		URL: session.URL,
		Metadata: map[string]any{
			"alipay_pay_url": session.URL,
			"alipay_mode":    payReq.Mode,
		},
	}, nil
}

func (p Provider) Refund(req checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	refundReq, err := alipayRefundRequest(req)
	if err != nil {
		return checkoutprovider.RefundResult{}, err
	}
	result, err := p.createRefund(refundReq)
	if err != nil {
		return checkoutprovider.RefundResult{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	return checkoutprovider.RefundResult{
		ID:       firstNonEmpty(result.TradeNo, req.IdempotencyKey),
		Status:   checkoutprovider.RefundStatusSucceeded,
		Metadata: map[string]any{"alipay_refund_fee": result.RefundFee, "alipay_fund_change": result.FundChange},
	}, nil
}

func (p Provider) QueryOrder(req checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	queryReq, err := alipayQueryRequest(req)
	if err != nil {
		return checkoutprovider.QueryResult{}, err
	}
	result, err := p.queryTrade(queryReq)
	if err != nil {
		return checkoutprovider.QueryResult{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	return checkoutprovider.QueryResult{
		Status:                alipayQueryStatus(result.TradeStatus),
		ProviderTransactionID: result.TradeNo,
		Amount:                normalizeAlipayAmount(result.TotalAmount),
		Currency:              "CNY",
		Metadata:              map[string]any{"alipay_trade_status": string(result.TradeStatus)},
	}, nil
}

func (p Provider) createPagePay(req PagePayRequest) (PagePaySession, error) {
	if p.Creator != nil {
		return p.Creator.CreatePagePay(req)
	}
	return createPagePay(req)
}

func createPagePay(req PagePayRequest) (PagePaySession, error) {
	client, err := newClient(req)
	if err != nil {
		return PagePaySession{}, err
	}
	trade := alipay.Trade{
		NotifyURL:      req.NotifyURL,
		ReturnURL:      req.ReturnURL,
		Subject:        req.Subject,
		OutTradeNo:     req.OrderNo,
		TotalAmount:    req.Amount,
		ProductCode:    "FAST_INSTANT_TRADE_PAY",
		Body:           req.Body,
		PassbackParams: url.QueryEscape(req.OrderNo),
	}
	if req.Mode == "wap" || req.Mode == "h5" {
		trade.ProductCode = "QUICK_WAP_WAY"
		payURL, err := client.TradeWapPay(alipay.TradeWapPay{Trade: trade})
		if err != nil {
			return PagePaySession{}, err
		}
		return PagePaySession{URL: payURL.String()}, nil
	}
	payURL, err := client.TradePagePay(alipay.TradePagePay{
		Trade:       trade,
		QRPayMode:   req.QRPayMode,
		QRCodeWidth: req.QRCodeWidth,
	})
	if err != nil {
		return PagePaySession{}, err
	}
	return PagePaySession{URL: payURL.String()}, nil
}

func (p Provider) createRefund(req alipayRefundRequestData) (*alipay.TradeRefundRsp, error) {
	if p.Creator != nil {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	client, err := newClient(req.PagePayRequest)
	if err != nil {
		return nil, err
	}
	return client.TradeRefund(context.Background(), alipay.TradeRefund{
		OutTradeNo:   req.OrderNo,
		TradeNo:      req.TradeNo,
		RefundAmount: req.Amount,
		RefundReason: req.Reason,
		OutRequestNo: req.OutRequestNo,
	})
}

func (p Provider) queryTrade(req alipayQueryRequestData) (*alipay.TradeQueryRsp, error) {
	if p.Creator != nil {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	client, err := newClient(req.PagePayRequest)
	if err != nil {
		return nil, err
	}
	return client.TradeQuery(context.Background(), alipay.TradeQuery{
		OutTradeNo: req.OrderNo,
		TradeNo:    req.TradeNo,
	})
}

type alipayRefundRequestData struct {
	PagePayRequest
	TradeNo      string
	Amount       string
	Reason       string
	OutRequestNo string
}

type alipayQueryRequestData struct {
	PagePayRequest
	TradeNo string
}

func newClient(req PagePayRequest) (*alipay.Client, error) {
	opts := []alipay.OptionFunc{}
	if req.GatewayURL != "" {
		if req.Production {
			opts = append(opts, alipay.WithProductionGateway(req.GatewayURL))
		} else {
			opts = append(opts, alipay.WithSandboxGateway(req.GatewayURL))
		}
	}
	client, err := alipay.New(req.AppID, req.PrivateKey, req.Production, opts...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.AlipayPublicKey) != "" {
		if err := client.LoadAliPayPublicKey(req.AlipayPublicKey); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func pagePayRequest(req checkoutprovider.Request) (PagePayRequest, error) {
	appID := configString(req.Config, "app_id", "appId")
	privateKey := configString(req.Config, "private_key", "app_private_key")
	notifyURL := configString(req.Config, "notify_url", "webhook_url")
	returnURL := configString(req.Config, "return_url", "success_url")
	if appID == "" || privateKey == "" || notifyURL == "" || returnURL == "" {
		return PagePayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	mode := strings.ToLower(configString(req.Config, "mode", "pay_mode"))
	if mode == "" {
		mode = methodMode(req.Metadata)
	}
	if mode == "" {
		mode = "page"
	}
	if mode != "page" && mode != "pc" && mode != "wap" && mode != "h5" {
		return PagePayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	amount := money2(req.Amount)
	if amount == "" {
		return PagePayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	returnURL = appendOrderNo(returnURL, req.OrderNo)
	if returnURL == "" {
		return PagePayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	return PagePayRequest{
		AppID:           appID,
		PrivateKey:      privateKey,
		AlipayPublicKey: configString(req.Config, "alipay_public_key", "public_key"),
		Production:      configBool(req.Config, "production", "prod"),
		GatewayURL:      configString(req.Config, "gateway_url", "api_gateway"),
		Mode:            mode,
		OrderNo:         req.OrderNo,
		Amount:          amount,
		Subject:         firstNonEmpty(configString(req.Config, "subject", "title"), "SRapi order "+req.OrderNo),
		Body:            configString(req.Config, "body", "description"),
		NotifyURL:       notifyURL,
		ReturnURL:       returnURL,
		QRPayMode:       configString(req.Config, "qr_pay_mode"),
		QRCodeWidth:     configString(req.Config, "qrcode_width", "qr_code_width"),
	}, nil
}

func alipayRefundRequest(req checkoutprovider.RefundRequest) (alipayRefundRequestData, error) {
	payReq, err := pagePayRequest(checkoutprovider.Request{
		Config:   req.Config,
		OrderNo:  req.OrderNo,
		Amount:   req.OriginalAmount,
		Currency: req.Currency,
		Metadata: req.Metadata,
	})
	if err != nil {
		return alipayRefundRequestData{}, err
	}
	amount := money2(req.Amount)
	if amount == "" {
		return alipayRefundRequestData{}, checkoutprovider.ErrInvalidConfig
	}
	return alipayRefundRequestData{
		PagePayRequest: payReq,
		TradeNo:        req.ProviderTransactionID,
		Amount:         amount,
		Reason:         strings.TrimSpace(req.Reason),
		OutRequestNo:   firstNonEmpty(req.IdempotencyKey, req.OrderNo),
	}, nil
}

func alipayQueryRequest(req checkoutprovider.QueryRequest) (alipayQueryRequestData, error) {
	payReq, err := pagePayRequest(checkoutprovider.Request{
		Config:   req.Config,
		OrderNo:  req.OrderNo,
		Amount:   firstNonEmpty(req.Amount, "1.00"),
		Currency: req.Currency,
		Metadata: req.Metadata,
	})
	if err != nil {
		return alipayQueryRequestData{}, err
	}
	return alipayQueryRequestData{PagePayRequest: payReq, TradeNo: req.ProviderTransactionID}, nil
}

func alipayQueryStatus(status alipay.TradeStatus) checkoutprovider.QueryStatus {
	switch status {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
		return checkoutprovider.QueryStatusPaid
	case alipay.TradeStatusClosed:
		return checkoutprovider.QueryStatusCanceled
	default:
		return checkoutprovider.QueryStatusPending
	}
}

func normalizeAlipayAmount(value string) string {
	if value == "" {
		return ""
	}
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

func methodMode(metadata map[string]any) string {
	method := strings.ToLower(strings.TrimSpace(mapString(metadata, "method", "payment_method")))
	switch method {
	case "alipay_wap", "alipay_h5", "h5":
		return "wap"
	default:
		return "page"
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

func money2(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return ""
	}
	rat := new(big.Rat)
	if _, ok := rat.SetString(value); !ok || rat.Sign() <= 0 {
		return ""
	}
	cents := new(big.Rat).Mul(rat, new(big.Rat).SetInt64(100))
	if !cents.IsInt() {
		return ""
	}
	valueInt := cents.Num()
	if !valueInt.IsInt64() || valueInt.Sign() <= 0 {
		return ""
	}
	centsValue := valueInt.Int64()
	return fmt.Sprintf("%d.%02d", centsValue/100, centsValue%100)
}

func configString(values map[string]any, keys ...string) string {
	return mapString(values, keys...)
}

func configBool(values map[string]any, keys ...string) bool {
	raw := strings.ToLower(mapString(values, keys...))
	return raw == "true" || raw == "1" || raw == "yes" || raw == "production"
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
		case bool:
			return strconv.FormatBool(typed)
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
