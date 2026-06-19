package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usermemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestCurrentUserAuthIdentitiesListsEmailIdentity(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/auth-identities", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated identities 401, got %d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/auth-identities", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected identities 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.CurrentUserAuthIdentityListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode identities response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one derived email identity, got %+v", resp.Data)
	}
	identity := resp.Data[0]
	if identity.Provider != apiopenapi.AuthIdentityProviderEmail {
		t.Fatalf("expected email provider, got %+v", identity.Provider)
	}
	if identity.UserId != loginResp.Data.User.Id || identity.ProviderKey != "local" {
		t.Fatalf("unexpected email identity owner/key: %+v", identity)
	}
	if identity.External || identity.CanUnbind {
		t.Fatalf("expected local email identity to be non-external and not unbindable, got %+v", identity)
	}
	if identity.Email == nil || *identity.Email != loginResp.Data.User.Email {
		t.Fatalf("expected email %q, got %+v", loginResp.Data.User.Email, identity.Email)
	}
	if identity.SubjectHint == nil || *identity.SubjectHint != string(loginResp.Data.User.Email) {
		t.Fatalf("expected subject hint from email, got %+v", identity.SubjectHint)
	}
}

func TestCurrentUserAuthIdentityUnbindRequiresCSRFAndRemovesExternalIdentity(t *testing.T) {
	userStore := usermemory.New()
	handler := New(config.Load(), nil, WithUserStore(userStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	userID := apiIDToIntForTest(t, loginResp.Data.User.Id)
	identity, err := userStore.UpsertAuthIdentity(context.Background(), userscontract.CreateUserAuthIdentity{
		UserID:              userID,
		Provider:            userscontract.AuthIdentityProviderLinuxDo,
		ProviderKey:         "linuxdo",
		ProviderSubjectHash: "sha256:linuxdo-subject",
		SubjectHint:         "linuxdo-user",
		DisplayName:         "LinuxDo User",
	})
	if err != nil {
		t.Fatalf("upsert external identity: %v", err)
	}
	path := "/api/v1/me/auth-identities/" + strconv.Itoa(identity.ID)

	missingCSRFReq := httptest.NewRequest(http.MethodDelete, path, nil)
	missingCSRFReq.AddCookie(sessionCookie)
	missingCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRec, missingCSRFReq)
	if missingCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", missingCSRFRec.Code, missingCSRFRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected unbind 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.CurrentUserAuthIdentityListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode unbind response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Provider != apiopenapi.AuthIdentityProviderEmail {
		t.Fatalf("expected only local email identity after unbind, got %+v", resp.Data)
	}
}

func apiIDToIntForTest(t *testing.T, id apiopenapi.Id) int {
	t.Helper()
	out, err := strconv.Atoi(string(id))
	if err != nil || out <= 0 {
		t.Fatalf("invalid api id %q", id)
	}
	return out
}
