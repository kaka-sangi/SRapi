package entstore

import (
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	accountstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/accounts"
	apikeystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/apikeys"
	auditstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/audit"
	billingstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/billing"
	eventsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/events"
	modelstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/models"
	paymentstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/payments"
	providerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/providers"
	schedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/scheduler"
	subscriptionstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/subscriptions"
	usagestore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/usage"
	userstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/users"
)

var ErrInvalidClient = errors.New("invalid ent store client")

type Stores struct {
	Users     userscontract.Store
	APIKeys   apikeycontract.Store
	Providers providercontract.Store
	Models    modelcontract.Store
	Accounts  accountcontract.Store
	Audit     auditcontract.Store
	Billing   billingcontract.Store
	Events    eventscontract.Store
	Payments  paymentcontract.Store
	Scheduler schedulercontract.Store
	Subscriptions subscriptioncontract.Store
	Usage     usagecontract.Store
}

func New(client *ent.Client) (Stores, error) {
	if client == nil {
		return Stores{}, ErrInvalidClient
	}
	users, err := userstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	apiKeys, err := apikeystore.New(client)
	if err != nil {
		return Stores{}, err
	}
	providers, err := providerstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	models, err := modelstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	accounts, err := accountstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	audit, err := auditstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	billing, err := billingstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	events, err := eventsstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	payments, err := paymentstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	scheduler, err := schedulerstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	subscriptions, err := subscriptionstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	usage, err := usagestore.New(client)
	if err != nil {
		return Stores{}, err
	}
	return Stores{
		Users:     users,
		APIKeys:   apiKeys,
		Providers: providers,
		Models:    models,
		Accounts:  accounts,
		Audit:     audit,
		Billing:   billing,
		Events:    events,
		Payments:  payments,
		Scheduler: scheduler,
		Subscriptions: subscriptions,
		Usage:     usage,
	}, nil
}
