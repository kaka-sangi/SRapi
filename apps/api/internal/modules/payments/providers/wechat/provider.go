package wechatprovider

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	wechatpayments "github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const (
	defaultWechatCurrency = "CNY"
	defaultWechatTimeout  = 15 * time.Second
)

type PrepayRequest struct {
	AppID               string
	MchID               string
	APIV3Key            string
	CertificateSerialNo string
	PrivateKey          string
	Mode                string
	OrderNo             string
	Amount              int64
	Currency            string
	Description         string
	NotifyURL           string
	PayerClientIP       string
	PayerOpenID         string
	H5Type              string
	H5AppName           string
	H5AppURL            string
	H5BundleID          string
	H5PackageName       string
}

type PrepaySession struct {
	ID       string
	URL      string
	Metadata map[string]any
}

type PrepayCreator interface {
	CreatePrepay(req PrepayRequest) (PrepaySession, error)
}

type Provider struct {
	Creator PrepayCreator
}

type Notification struct {
	EventID       string
	EventType     string
	OrderNo       string
	TransactionID string
	TradeState    string
	Amount        string
	Currency      string
	Payload       map[string]any
}

func New() Provider {
	return Provider{}
}

func (p Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	prepayReq, err := prepayRequest(req)
	if err != nil {
		return checkoutprovider.Session{}, err
	}
	session, err := p.createPrepay(prepayReq)
	if err != nil {
		return checkoutprovider.Session{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	if prepayReq.Mode != "jsapi" && strings.TrimSpace(session.URL) == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	if prepayReq.Mode == "jsapi" && strings.TrimSpace(session.ID) == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	metadata := map[string]any{
		"wechat_pay_mode": prepayReq.Mode,
	}
	if session.URL != "" {
		metadata["wechat_pay_url"] = session.URL
	}
	if session.ID != "" {
		metadata["wechat_prepay_id"] = session.ID
	}
	for key, value := range session.Metadata {
		metadata[key] = value
	}
	return checkoutprovider.Session{
		ID:       firstNonEmpty(session.ID, req.OrderNo),
		URL:      session.URL,
		Metadata: metadata,
	}, nil
}

func (Provider) CreatePrepay(req PrepayRequest) (PrepaySession, error) {
	privateKey, err := loadPrivateKey(req.PrivateKey)
	if err != nil {
		return PrepaySession{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWechatTimeout)
	defer cancel()
	client, err := core.NewClient(ctx, option.WithWechatPayAutoAuthCipher(req.MchID, req.CertificateSerialNo, privateKey, req.APIV3Key))
	if err != nil {
		return PrepaySession{}, err
	}
	switch req.Mode {
	case "h5":
		resp, _, err := (&h5.H5ApiService{Client: client}).Prepay(ctx, h5.PrepayRequest{
			Appid:       core.String(req.AppID),
			Mchid:       core.String(req.MchID),
			Description: core.String(req.Description),
			OutTradeNo:  core.String(req.OrderNo),
			NotifyUrl:   core.String(req.NotifyURL),
			Amount: &h5.Amount{
				Currency: core.String(req.Currency),
				Total:    core.Int64(req.Amount),
			},
			SceneInfo: &h5.SceneInfo{
				PayerClientIp: core.String(req.PayerClientIP),
				H5Info: &h5.H5Info{
					Type:        core.String(req.H5Type),
					AppName:     optionalCoreString(req.H5AppName),
					AppUrl:      optionalCoreString(req.H5AppURL),
					BundleId:    optionalCoreString(req.H5BundleID),
					PackageName: optionalCoreString(req.H5PackageName),
				},
			},
		})
		if err != nil {
			return PrepaySession{}, err
		}
		return PrepaySession{ID: req.OrderNo, URL: stringValue(resp.H5Url), Metadata: map[string]any{"wechat_h5_url": stringValue(resp.H5Url)}}, nil
	case "jsapi":
		resp, _, err := (&jsapi.JsapiApiService{Client: client}).PrepayWithRequestPayment(ctx, jsapi.PrepayRequest{
			Appid:       core.String(req.AppID),
			Mchid:       core.String(req.MchID),
			Description: core.String(req.Description),
			OutTradeNo:  core.String(req.OrderNo),
			NotifyUrl:   core.String(req.NotifyURL),
			Amount: &jsapi.Amount{
				Currency: core.String(req.Currency),
				Total:    core.Int64(req.Amount),
			},
			Payer: &jsapi.Payer{Openid: core.String(req.PayerOpenID)},
		})
		if err != nil {
			return PrepaySession{}, err
		}
		return PrepaySession{
			ID: stringValue(resp.PrepayId),
			Metadata: map[string]any{
				"wechat_app_id":    stringValue(resp.Appid),
				"wechat_nonce_str": stringValue(resp.NonceStr),
				"wechat_package":   stringValue(resp.Package),
				"wechat_pay_sign":  stringValue(resp.PaySign),
				"wechat_prepay_id": stringValue(resp.PrepayId),
				"wechat_sign_type": stringValue(resp.SignType),
				"wechat_timestamp": stringValue(resp.TimeStamp),
			},
		}, nil
	default:
		resp, _, err := (&native.NativeApiService{Client: client}).Prepay(ctx, native.PrepayRequest{
			Appid:       core.String(req.AppID),
			Mchid:       core.String(req.MchID),
			Description: core.String(req.Description),
			OutTradeNo:  core.String(req.OrderNo),
			NotifyUrl:   core.String(req.NotifyURL),
			Amount: &native.Amount{
				Currency: core.String(req.Currency),
				Total:    core.Int64(req.Amount),
			},
			SceneInfo: nativeSceneInfo(req.PayerClientIP),
		})
		if err != nil {
			return PrepaySession{}, err
		}
		return PrepaySession{ID: req.OrderNo, URL: stringValue(resp.CodeUrl), Metadata: map[string]any{"wechat_code_url": stringValue(resp.CodeUrl)}}, nil
	}
}

func ParseNotification(rawBody string, headers map[string]string, config map[string]any) (Notification, error) {
	rawBody = strings.TrimSpace(rawBody)
	if rawBody == "" {
		return Notification{}, checkoutprovider.ErrInvalidConfig
	}
	handler, err := notifyHandler(config)
	if err != nil {
		return Notification{}, err
	}
	request, err := http.NewRequest(http.MethodPost, "https://srapi.local/api/v1/webhooks/payments/wechat", strings.NewReader(rawBody))
	if err != nil {
		return Notification{}, err
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		request.Header.Set(key, value)
	}
	transaction := new(wechatpayments.Transaction)
	notifyReq, err := handler.ParseNotifyRequest(context.Background(), request, transaction)
	if err != nil {
		return Notification{}, err
	}
	return notificationFromTransaction(notifyReq, transaction), nil
}

func (p Provider) createPrepay(req PrepayRequest) (PrepaySession, error) {
	if p.Creator != nil {
		return p.Creator.CreatePrepay(req)
	}
	return p.CreatePrepay(req)
}

func prepayRequest(req checkoutprovider.Request) (PrepayRequest, error) {
	appID := configString(req.Config, "app_id", "appid")
	mchID := configString(req.Config, "mch_id", "mchid", "merchant_id")
	apiV3Key := configString(req.Config, "api_v3_key", "apiV3Key")
	serialNo := configString(req.Config, "serial_no", "certificate_serial_no", "mch_certificate_serial_no")
	privateKey := configString(req.Config, "private_key", "merchant_private_key")
	notifyURL := configString(req.Config, "notify_url", "webhook_url")
	if appID == "" || mchID == "" || apiV3Key == "" || serialNo == "" || privateKey == "" || notifyURL == "" {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = defaultWechatCurrency
	}
	if currency != defaultWechatCurrency {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	amount, ok := minorAmount(req.Amount)
	if !ok || amount <= 0 {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	mode := paymentMode(req.Config, req.Metadata)
	out := PrepayRequest{
		AppID:               appID,
		MchID:               mchID,
		APIV3Key:            apiV3Key,
		CertificateSerialNo: serialNo,
		PrivateKey:          privateKey,
		Mode:                mode,
		OrderNo:             strings.TrimSpace(req.OrderNo),
		Amount:              amount,
		Currency:            currency,
		Description:         firstNonEmpty(configString(req.Config, "description", "body", "subject"), "SRapi order "+strings.TrimSpace(req.OrderNo)),
		NotifyURL:           notifyURL,
		PayerClientIP:       firstNonEmpty(metadataString(req.Metadata, "payer_client_ip", "client_ip"), configString(req.Config, "payer_client_ip", "client_ip")),
		PayerOpenID:         firstNonEmpty(metadataString(req.Metadata, "payer_openid", "openid"), configString(req.Config, "payer_openid", "openid")),
		H5Type:              firstNonEmpty(configString(req.Config, "h5_type", "scene_type"), "Wap"),
		H5AppName:           configString(req.Config, "h5_app_name", "app_name"),
		H5AppURL:            configString(req.Config, "h5_app_url", "app_url"),
		H5BundleID:          configString(req.Config, "h5_bundle_id", "bundle_id"),
		H5PackageName:       configString(req.Config, "h5_package_name", "package_name"),
	}
	if out.OrderNo == "" || out.Description == "" {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	if out.Mode == "h5" && out.PayerClientIP == "" {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	if out.Mode == "jsapi" && out.PayerOpenID == "" {
		return PrepayRequest{}, checkoutprovider.ErrInvalidConfig
	}
	return out, nil
}

func paymentMode(config map[string]any, metadata map[string]any) string {
	mode := strings.ToLower(configString(config, "mode", "pay_mode"))
	if mode == "" {
		mode = strings.ToLower(metadataString(metadata, "method", "payment_method"))
	}
	switch mode {
	case "h5", "wap", "wechat_h5", "wechat_wap":
		return "h5"
	case "jsapi", "wechat_jsapi", "wechat_mini_program":
		return "jsapi"
	default:
		return "native"
	}
}

func notifyHandler(config map[string]any) (*notify.Handler, error) {
	apiV3Key := configString(config, "api_v3_key", "apiV3Key")
	if apiV3Key == "" {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	verifier, err := notificationVerifier(config)
	if err != nil {
		return nil, err
	}
	return notify.NewRSANotifyHandler(apiV3Key, verifier)
}

func notificationVerifier(config map[string]any) (auth.Verifier, error) {
	publicKey := configString(config, "wechatpay_public_key", "public_key")
	publicKeyID := configString(config, "wechatpay_public_key_id", "public_key_id", "platform_public_key_id")
	if publicKey != "" || publicKeyID != "" {
		if publicKey == "" || publicKeyID == "" {
			return nil, checkoutprovider.ErrInvalidConfig
		}
		parsed, err := utils.LoadPublicKey(publicKey)
		if err != nil {
			return nil, err
		}
		return verifiers.NewSHA256WithRSAPubkeyVerifier(publicKeyID, *parsed), nil
	}
	mchID := configString(config, "mch_id", "mchid", "merchant_id")
	apiV3Key := configString(config, "api_v3_key", "apiV3Key")
	serialNo := configString(config, "serial_no", "certificate_serial_no", "mch_certificate_serial_no")
	privateKeyText := configString(config, "private_key", "merchant_private_key")
	if mchID == "" || apiV3Key == "" || serialNo == "" || privateKeyText == "" {
		return nil, checkoutprovider.ErrInvalidConfig
	}
	privateKey, err := loadPrivateKey(privateKeyText)
	if err != nil {
		return nil, err
	}
	mgr := downloader.MgrInstance()
	ctx, cancel := context.WithTimeout(context.Background(), defaultWechatTimeout)
	defer cancel()
	if !mgr.HasDownloader(ctx, mchID) {
		if err := mgr.RegisterDownloaderWithPrivateKey(ctx, privateKey, serialNo, mchID, apiV3Key); err != nil {
			return nil, err
		}
	}
	return verifiers.NewSHA256WithRSAVerifier(mgr.GetCertificateVisitor(mchID)), nil
}

func notificationFromTransaction(notifyReq *notify.Request, transaction *wechatpayments.Transaction) Notification {
	payload := map[string]any{
		"event_id":     notifyReq.ID,
		"event_type":   notifyReq.EventType,
		"out_trade_no": stringValue(transaction.OutTradeNo),
		"trade_state":  stringValue(transaction.TradeState),
		"currency":     defaultWechatCurrency,
	}
	if transaction.TransactionId != nil {
		payload["provider_transaction_id"] = *transaction.TransactionId
	}
	if transaction.Amount != nil {
		if transaction.Amount.Total != nil {
			payload["amount"] = amountFromMinor(*transaction.Amount.Total)
		}
		if transaction.Amount.Currency != nil {
			payload["currency"] = *transaction.Amount.Currency
		}
	}
	return Notification{
		EventID:       notifyReq.ID,
		EventType:     notifyReq.EventType,
		OrderNo:       stringValue(transaction.OutTradeNo),
		TransactionID: stringValue(transaction.TransactionId),
		TradeState:    stringValue(transaction.TradeState),
		Amount:        stringValueFromMap(payload, "amount"),
		Currency:      stringValueFromMap(payload, "currency"),
		Payload:       payload,
	}
}

func loadPrivateKey(privateKeyText string) (*rsa.PrivateKey, error) {
	privateKey, err := utils.LoadPrivateKey(privateKeyText)
	if err == nil {
		return privateKey, nil
	}
	block, _ := pem.Decode([]byte(privateKeyText))
	if block == nil {
		return nil, err
	}
	if block.Type != "RSA PRIVATE KEY" {
		return nil, err
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func nativeSceneInfo(payerClientIP string) *native.SceneInfo {
	payerClientIP = strings.TrimSpace(payerClientIP)
	if payerClientIP == "" {
		return nil
	}
	return &native.SceneInfo{PayerClientIp: core.String(payerClientIP)}
}

func minorAmount(amount string) (int64, bool) {
	rat, ok := money.RequiredDecimalRat(amount)
	if !ok || rat.Sign() <= 0 {
		return 0, false
	}
	rat.Mul(rat, new(big.Rat).SetInt64(100))
	if !rat.IsInt() {
		return 0, false
	}
	value := rat.Num()
	if !value.IsInt64() || value.Sign() <= 0 {
		return 0, false
	}
	return value.Int64(), true
}

func amountFromMinor(amount int64) string {
	whole := amount / 100
	fraction := amount % 100
	return strconv.FormatInt(whole, 10) + "." + leftPad(strconv.FormatInt(fraction, 10), 2) + "000000"
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
		case bool:
			return strconv.FormatBool(typed)
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringValueFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func optionalCoreString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return core.String(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func leftPad(value string, width int) string {
	for len(value) < width {
		value = "0" + value
	}
	return value
}
