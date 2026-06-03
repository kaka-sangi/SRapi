package entstore

import (
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	idempotencycontract "github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userplatformquotascontract "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	accountstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/accounts"
	admincontrolstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/admincontrol"
	affiliatestore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/affiliate"
	apikeystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/apikeys"
	auditstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/audit"
	authstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/auth"
	billingstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/billing"
	errorpassthroughstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/errorpassthrough"
	eventsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/events"
	groupratelimitsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/groupratelimits"
	healthrollupsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/healthrollups"
	idempotencystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/idempotency"
	modelratelimitsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/modelratelimits"
	modelstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/models"
	operationsstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/operations"
	paymentstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/payments"
	providerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/providers"
	qualitystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/qualityeval"
	schedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/scheduler"
	subscriptionstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/subscriptions"
	tlsprofilesstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/tlsprofiles"
	totpstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/totp"
	usagestore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/usage"
	userattributesstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/userattributes"
	userplatformquotasstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/userplatformquotas"
	userstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/users"
)

var ErrInvalidClient = errors.New("invalid ent store client")

type Stores struct {
	AdminControl       admincontrolcontract.Store
	Users              userscontract.Store
	APIKeys            apikeycontract.Store
	Affiliate          affiliatecontract.Store
	Providers          providercontract.Store
	Models             modelcontract.Store
	Accounts           accountcontract.Store
	Audit              auditcontract.Store
	AuthSessions       authcontract.Store
	Billing            billingcontract.Store
	UsageCharges       billingcontract.UsageChargeStore
	Events             eventscontract.Store
	Idempotency        idempotencycontract.Store
	Operations         operationscontract.Store
	Payments           paymentcontract.Store
	QualityEval        qualitycontract.Store
	Scheduler          schedulercontract.Store
	Subscriptions      subscriptioncontract.Store
	TOTP               totpcontract.Store
	Usage              usagecontract.Store
	UserAttributes     userattributescontract.Store
	ErrorPassthrough   errorpassthroughcontract.Store
	TLSProfiles        tlsprofilescontract.Store
	HealthRollups      healthrollupscontract.Store
	ModelRateLimits    modelratelimitscontract.Store
	GroupRateLimits    groupratelimitscontract.Store
	UserPlatformQuotas userplatformquotascontract.Store
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
	affiliate, err := affiliatestore.New(client)
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
	authSessions, err := authstore.New(client)
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
	operations, err := operationsstore.New(client)
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
	qualityEval, err := qualitystore.New(client)
	if err != nil {
		return Stores{}, err
	}
	subscriptions, err := subscriptionstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	totp, err := totpstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	usage, err := usagestore.New(client)
	if err != nil {
		return Stores{}, err
	}
	adminControl, err := admincontrolstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	idempotency, err := idempotencystore.New(client)
	if err != nil {
		return Stores{}, err
	}
	userAttributes, err := userattributesstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	errorPassthrough, err := errorpassthroughstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	tlsProfiles, err := tlsprofilesstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	healthRollups, err := healthrollupsstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	modelRateLimits, err := modelratelimitsstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	groupRateLimits, err := groupratelimitsstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	userPlatformQuotas, err := userplatformquotasstore.New(client)
	if err != nil {
		return Stores{}, err
	}
	return Stores{
		AdminControl:       adminControl,
		Users:              users,
		APIKeys:            apiKeys,
		Affiliate:          affiliate,
		Providers:          providers,
		Models:             models,
		Accounts:           accounts,
		Audit:              audit,
		AuthSessions:       authSessions,
		Billing:            billing,
		UsageCharges:       billing,
		Events:             events,
		Idempotency:        idempotency,
		Operations:         operations,
		Payments:           payments,
		QualityEval:        qualityEval,
		Scheduler:          scheduler,
		Subscriptions:      subscriptions,
		TOTP:               totp,
		Usage:              usage,
		UserAttributes:     userAttributes,
		ErrorPassthrough:   errorPassthrough,
		TLSProfiles:        tlsProfiles,
		HealthRollups:      healthRollups,
		ModelRateLimits:    modelRateLimits,
		GroupRateLimits:    groupRateLimits,
		UserPlatformQuotas: userPlatformQuotas,
	}, nil
}
