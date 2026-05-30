package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestNotificationUnsubscribeEndpointStoresOptionalPreference(t *testing.T) {
	cfg := config.Load()
	cfg.Email.PublicBaseURL = "https://console.srapi.local"
	store := admincontrolmemory.New()
	handler := New(cfg, nil, WithAdminControlStore(store))
	preferences, err := notificationsservice.NewPreferenceService(store, cfg.Security.MasterKey, cfg.Email.PublicBaseURL)
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	token, err := preferences.CreateUnsubscribeToken("User@Example.COM", notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unsubscribe?token="+token, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected preview 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var preview apiopenapi.NotificationUnsubscribeResponse
	if err := json.NewDecoder(getRec.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Data.Event != notificationscontract.TemplateBalanceLow || preview.Data.Done {
		t.Fatalf("unexpected preview response: %+v", preview.Data)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/unsubscribe?token="+token, strings.NewReader("List-Unsubscribe=One-Click"))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("expected unsubscribe 200, got %d body=%s", postRec.Code, postRec.Body.String())
	}
	var response apiopenapi.NotificationUnsubscribeResponse
	if err := json.NewDecoder(postRec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Event != notificationscontract.TemplateBalanceLow || !response.Data.Done {
		t.Fatalf("unexpected unsubscribe response: %+v", response.Data)
	}
	unsubscribed, err := preferences.IsUnsubscribed(context.Background(), "user@example.com", notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check preference: %v", err)
	}
	if !unsubscribed {
		t.Fatal("expected preference to be stored by route")
	}
}

func TestNotificationUnsubscribeEndpointRejectsInvalidToken(t *testing.T) {
	cfg := config.Load()
	cfg.Email.PublicBaseURL = "https://console.srapi.local"
	handler := New(cfg, nil, WithAdminControlStore(admincontrolmemory.New()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/unsubscribe", strings.NewReader(`{"token":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid token 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCurrentUserNotificationPreferencesListUpdateAndCSRF(t *testing.T) {
	cfg := config.Load()
	cfg.Email.PublicBaseURL = "https://console.srapi.local"
	store := admincontrolmemory.New()
	handler := New(cfg, nil, WithAdminControlStore(store))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/notification-preferences", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected preferences list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.NotificationPreferenceListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 3 {
		t.Fatalf("expected three optional preferences, got %+v", listResp.Data)
	}
	for _, item := range listResp.Data {
		if !item.Subscribed {
			t.Fatalf("new user preference should default to subscribed: %+v", listResp.Data)
		}
	}

	noCSRFReq := httptest.NewRequest(http.MethodPut, "/api/v1/me/notification-preferences", strings.NewReader(`{"preferences":[{"event":"balance.low","subscribed":false}]}`))
	noCSRFReq.Header.Set("Content-Type", "application/json")
	noCSRFReq.AddCookie(sessionCookie)
	noCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(noCSRFRec, noCSRFReq)
	if noCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", noCSRFRec.Code, noCSRFRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/me/notification-preferences", strings.NewReader(`{"preferences":[{"event":"balance.low","subscribed":false}]}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected preferences update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp apiopenapi.NotificationPreferenceListResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if !hasNotificationPreference(updateResp.Data, notificationscontract.TemplateBalanceLow, false) {
		t.Fatalf("expected balance.low to be unsubscribed, got %+v", updateResp.Data)
	}

	preferences, err := notificationsservice.NewPreferenceService(store, cfg.Security.MasterKey, cfg.Email.PublicBaseURL)
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	unsubscribed, err := preferences.IsUnsubscribed(context.Background(), "admin@srapi.local", notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check stored preference: %v", err)
	}
	if !unsubscribed {
		t.Fatal("current-user preference update should share optional preference store")
	}

	resubscribeReq := httptest.NewRequest(http.MethodPut, "/api/v1/me/notification-preferences", strings.NewReader(`{"preferences":[{"event":"balance.low","subscribed":true}]}`))
	resubscribeReq.Header.Set("Content-Type", "application/json")
	resubscribeReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	resubscribeReq.AddCookie(sessionCookie)
	resubscribeRec := httptest.NewRecorder()
	handler.ServeHTTP(resubscribeRec, resubscribeReq)
	if resubscribeRec.Code != http.StatusOK {
		t.Fatalf("expected preferences resubscribe 200, got %d body=%s", resubscribeRec.Code, resubscribeRec.Body.String())
	}
	unsubscribed, err = preferences.IsUnsubscribed(context.Background(), "admin@srapi.local", notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check resubscribed preference: %v", err)
	}
	if unsubscribed {
		t.Fatal("resubscribe should clear optional suppression state")
	}
}

func TestCurrentUserNotificationPreferencesRejectsUnsupportedEvent(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/me/notification-preferences", strings.NewReader(`{"preferences":[{"event":"auth.password_reset","subscribed":false}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported preference event 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCurrentUserNotificationContactsVerificationLifecycle(t *testing.T) {
	cfg := config.Load()
	cfg.Email.PublicBaseURL = "https://console.srapi.local"
	adminStore := admincontrolmemory.New()
	eventStore := eventsmemory.New()
	handler := New(cfg, nil, WithAdminControlStore(adminStore), WithEventStore(eventStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	noCSRFReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/notification-contacts", strings.NewReader(`{"email":"alerts@srapi.local"}`))
	noCSRFReq.Header.Set("Content-Type", "application/json")
	noCSRFReq.AddCookie(sessionCookie)
	noCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(noCSRFRec, noCSRFReq)
	if noCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", noCSRFRec.Code, noCSRFRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/notification-contacts", strings.NewReader(`{"email":"Alerts@SRapi.Local"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected contact verification 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.NotificationContactVerificationResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create contact response: %v", err)
	}
	if !createResp.Data.Accepted || !createResp.Data.VerificationSent || createResp.Data.Contact.Email != "alerts@srapi.local" || createResp.Data.Contact.Verified {
		t.Fatalf("unexpected create contact response: %+v", createResp.Data)
	}

	events, err := eventsservice.New(eventStore, nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	outbox, err := events.ListOutbox(context.Background())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != notificationscontract.EventNotificationContactVerificationRequested {
		t.Fatalf("expected contact verification event, got %+v", outbox)
	}
	if strings.Contains(strings.ToLower(outbox[0].IdempotencyKey), "alerts@srapi.local") || strings.Contains(strings.ToLower(outboxPayloadString(outbox[0].Payload)), "alerts@srapi.local") {
		t.Fatalf("outbox should not contain plaintext contact email: %+v", outbox[0].Payload)
	}
	token, err := notificationsservice.DecryptNotificationContactSecret(cfg.Security.MasterKey, strings.TrimSpace(outbox[0].Payload["verification_token_ciphertext"].(string)), notificationsservice.NotificationContactTokenAAD())
	if err != nil {
		t.Fatalf("decrypt verification token: %v", err)
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/notification-contacts/verify", strings.NewReader(`{"token":"`+token+`"}`))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	confirmReq.AddCookie(sessionCookie)
	confirmRec := httptest.NewRecorder()
	handler.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("expected contact verify 200, got %d body=%s", confirmRec.Code, confirmRec.Body.String())
	}
	var confirmResp apiopenapi.NotificationContactResponse
	if err := json.NewDecoder(confirmRec.Body).Decode(&confirmResp); err != nil {
		t.Fatalf("decode confirm contact response: %v", err)
	}
	if !confirmResp.Data.Verified || confirmResp.Data.VerifiedAt == nil {
		t.Fatalf("expected verified contact, got %+v", confirmResp.Data)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/me/notification-contacts/"+confirmResp.Data.Id, strings.NewReader(`{"disabled":true}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected contact update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp apiopenapi.NotificationContactResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update contact response: %v", err)
	}
	if !updateResp.Data.Disabled {
		t.Fatalf("expected disabled contact, got %+v", updateResp.Data)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/me/notification-contacts/"+confirmResp.Data.Id, nil)
	deleteReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected contact delete 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func hasNotificationPreference(items []apiopenapi.NotificationPreference, event string, subscribed bool) bool {
	for _, item := range items {
		if string(item.Event) == event && item.Subscribed == subscribed {
			return true
		}
	}
	return false
}

func outboxPayloadString(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	return string(raw)
}
