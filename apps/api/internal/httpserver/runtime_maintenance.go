package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// maintenanceGateMiddleware short-circuits gateway traffic (/v1/* and /v1beta/*)
// with a structured 503 while the admin-toggled maintenance flag is on. All
// other surfaces — the admin console, auth, health probes, payment webhooks —
// pass through so operators can disable the gate from the running process.
//
// The check is path-prefixed and reads the cached admin-settings snapshot, so
// the steady-state cost is a single map lookup per request. Errors from the
// settings store fail-open: an unhealthy settings backend must not silently
// 503 the whole gateway.
func (s *Server) maintenanceGateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !maintenanceGateAppliesTo(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
		if err != nil || !settings.Maintenance.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		writeMaintenanceResponse(w, r, settings.Maintenance)
	})
}

// maintenanceGateAppliesTo flags the paths the gate covers. Only the
// public gateway namespaces are in scope; the admin console and auth surface
// remain reachable so the maintenance flag can be flipped off.
func maintenanceGateAppliesTo(path string) bool {
	return strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/") || path == "/v1" || path == "/v1beta"
}

func writeMaintenanceResponse(w http.ResponseWriter, r *http.Request, m admincontrolcontract.AdminSettingsMaintenance) {
	message := strings.TrimSpace(m.Message)
	if message == "" {
		message = "service is in maintenance"
	}
	if m.ExpectedRecoveryAt != nil {
		if delta := time.Until(*m.ExpectedRecoveryAt); delta > 0 {
			seconds := int((delta + time.Second - time.Nanosecond) / time.Second)
			if seconds > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
			}
			message += " (estimated recovery: " + m.ExpectedRecoveryAt.UTC().Format(time.RFC3339) + ")"
		}
	}
	if strings.HasPrefix(r.URL.Path, "/v1beta/") {
		writeGeminiGatewayError(w, http.StatusServiceUnavailable, "UNAVAILABLE", message)
		return
	}
	writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, message, "maintenance")
}
