package service

import (
	alipayprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/alipay"
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	easypayprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/easypay"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
	wechatprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/wechat"
)

func defaultCheckoutRegistry(stripe stripeprovider.CheckoutCreator) checkoutprovider.Registry {
	return checkoutprovider.Registry{
		"alipay":  alipayprovider.New(),
		"easypay": easypayprovider.New(),
		"stripe":  stripeCheckoutAdapter{creator: stripe},
		"wechat":  wechatprovider.New(),
	}
}

type stripeCheckoutAdapter struct {
	creator stripeprovider.CheckoutCreator
}

func (a stripeCheckoutAdapter) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	if a.creator == nil {
		return checkoutprovider.Session{}, ErrProviderUnavailable
	}
	return stripeprovider.Provider{Creator: a.creator}.CreateSession(req)
}
