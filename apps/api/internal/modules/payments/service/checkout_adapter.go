package service

import (
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	easypayprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/easypay"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
)

func defaultCheckoutRegistry(stripe stripeprovider.CheckoutCreator) checkoutprovider.Registry {
	return checkoutprovider.Registry{
		"easypay": easypayprovider.New(),
		"stripe":  stripeCheckoutAdapter{creator: stripe},
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
