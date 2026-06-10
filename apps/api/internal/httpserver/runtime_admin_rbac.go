package httpserver

import (
	"net/http"
	"strings"

	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) adminRBACMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		permission := adminRoutePermission(r.Method, r.URL.Path)
		if permission == "" {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := s.requireAdminPermission(r, permission); err != nil {
			requestID := requestIDFromContext(r.Context())
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "permission required", requestID)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func adminRoutePermission(method string, path string) string {
	if !strings.HasPrefix(path, "/api/v1/admin/") {
		return ""
	}
	resource := adminRouteResource(path)
	if resource == "" {
		return ""
	}
	action := "write"
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		action = "read"
	}
	return resource + ":" + action
}

func adminRouteResource(path string) string {
	switch {
	case path == "/api/v1/admin/overview", strings.HasPrefix(path, "/api/v1/admin/dashboard"):
		return "dashboard"
	case strings.HasPrefix(path, "/api/v1/admin/permission-catalog"):
		return "role"
	case strings.HasPrefix(path, "/api/v1/admin/roles"):
		return "role"
	case strings.HasPrefix(path, "/api/v1/admin/user-attributes"), strings.Contains(path, "/attributes"):
		return "user_attribute"
	case strings.Contains(path, "/platform-quotas"):
		return "user_platform_quota"
	case strings.HasPrefix(path, "/api/v1/admin/users"):
		return "user"
	case strings.HasPrefix(path, "/api/v1/admin/api-keys"):
		return "api_key"
	case strings.HasPrefix(path, "/api/v1/admin/providers"):
		return "provider"
	case strings.HasPrefix(path, "/api/v1/admin/quick-setup"):
		return "provider"
	case strings.HasPrefix(path, "/api/v1/admin/models"):
		return "model"
	case strings.HasPrefix(path, "/api/v1/admin/accounts/health-summary"), strings.Contains(path, "/health"), strings.Contains(path, "/quota"), strings.Contains(path, "/rpm-status"), strings.Contains(path, "/proxy-quality"):
		return "account"
	case strings.HasPrefix(path, "/api/v1/admin/accounts"):
		return "account"
	case strings.HasPrefix(path, "/api/v1/admin/account-groups"):
		return "account_group"
	case strings.HasPrefix(path, "/api/v1/admin/proxies"):
		return "proxy"
	case strings.HasPrefix(path, "/api/v1/admin/usage"):
		return "usage"
	case strings.HasPrefix(path, "/api/v1/admin/audit-logs"):
		return "audit_log"
	case strings.HasPrefix(path, "/api/v1/admin/billing-ledger"):
		return "billing_ledger"
	case strings.HasPrefix(path, "/api/v1/admin/affiliates"), strings.HasPrefix(path, "/api/v1/admin/affiliate-rules"):
		return "affiliate"
	case strings.HasPrefix(path, "/api/v1/admin/payments/providers"):
		return "payment_provider"
	case strings.HasPrefix(path, "/api/v1/admin/payments/orders"):
		return "payment"
	case strings.HasPrefix(path, "/api/v1/admin/subscription-plans"), strings.HasPrefix(path, "/api/v1/admin/user-subscriptions"):
		return "subscription"
	case strings.HasPrefix(path, "/api/v1/admin/pricing-rules"):
		return "pricing_rule"
	case strings.HasPrefix(path, "/api/v1/admin/settings"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/admin/notifications"):
		return "notification_template"
	case strings.HasPrefix(path, "/api/v1/admin/risk-control"):
		return "risk_control"
	case strings.HasPrefix(path, "/api/v1/admin/capabilities"):
		return "capability"
	case strings.HasPrefix(path, "/api/v1/admin/scheduler"):
		return "scheduler"
	case strings.HasPrefix(path, "/api/v1/admin/alert-rules"), strings.Contains(path, "/alert-rules"), strings.Contains(path, "/alert-silences"):
		return "alert_rule"
	case strings.HasPrefix(path, "/api/v1/admin/ops"):
		return "ops"
	case strings.HasPrefix(path, "/api/v1/admin/tls-profiles"):
		return "tls_profile"
	case strings.HasPrefix(path, "/api/v1/admin/payload-rules"):
		return "payload_rule"
	case strings.HasPrefix(path, "/api/v1/admin/error-passthrough"):
		return "error_passthrough"
	case strings.HasPrefix(path, "/api/v1/admin/model-rate-limits"):
		return "model_rate_limit"
	case strings.HasPrefix(path, "/api/v1/admin/group-rate-limits"):
		return "group_rate_limit"
	case strings.HasPrefix(path, "/api/v1/admin/user-platform-quotas"):
		return "user_platform_quota"
	case strings.HasPrefix(path, "/api/v1/admin/channel-monitors"):
		return "channel_monitor"
	case strings.HasPrefix(path, "/api/v1/admin/channel-monitor-templates"):
		return "channel_monitor"
	case strings.HasPrefix(path, "/api/v1/admin/scheduled-tests"), strings.HasPrefix(path, "/api/v1/admin/scheduled-test-plans"):
		return "scheduled_test"
	case strings.HasPrefix(path, "/api/v1/admin/config-snapshot"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/admin/copilot"):
		return "copilot"
	case strings.HasPrefix(path, "/api/v1/admin/announcements"):
		return "announcement"
	case strings.HasPrefix(path, "/api/v1/admin/redeem-codes"):
		return "redeem_code"
	case strings.HasPrefix(path, "/api/v1/admin/promo-codes"):
		return "promo_code"
	case strings.HasPrefix(path, "/api/v1/admin/content-safety"):
		return "content_safety"
	default:
		return ""
	}
}

func (s *Server) handleAdminPermissionCatalog(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminPermission(r, userscontract.PermissionRoleRead); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "permission required", requestID)
		return
	}
	items := userscontract.PermissionCatalog()
	data := make([]apiopenapi.PermissionDefinition, 0, len(items))
	for _, item := range items {
		data = append(data, apiopenapi.PermissionDefinition{
			Action:      apiopenapi.PermissionDefinitionAction(item.Action),
			Description: item.Description,
			Permission:  item.Permission,
			Resource:    item.Resource,
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PermissionCatalogResponse{
		Data:      data,
		RequestId: requestID,
	})
}
