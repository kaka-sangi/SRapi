package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestCurrentUserAnnouncementsListAndReadState(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	allAnnouncement := mustCreateAdminAnnouncement(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"title":"Visible","content":"visible to everyone","status":"published","audience":"all","severity":"info"}`)
	hiddenAnnouncement := mustCreateAdminAnnouncement(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"title":"Users only","content":"hidden from admins","status":"published","audience":"users","severity":"warning"}`)
	mustCreateAdminAnnouncement(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"title":"Draft","content":"not published","status":"draft","audience":"all","severity":"info"}`)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/announcements", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.UserAnnouncementListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Unread != 1 {
		t.Fatalf("expected one unread visible announcement, got %+v", listResp)
	}
	if listResp.Data[0].Announcement.Id != allAnnouncement.Data.Id || listResp.Data[0].Read {
		t.Fatalf("unexpected visible announcement item: %+v", listResp.Data[0])
	}

	readReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/announcements/"+string(allAnnouncement.Data.Id)+"/read", nil)
	readReq.AddCookie(sessionCookie)
	readReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}
	var readResp apiopenapi.UserAnnouncementResponse
	if err := json.NewDecoder(readRec.Body).Decode(&readResp); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if !readResp.Data.Read || readResp.Data.ReadAt == nil {
		t.Fatalf("expected read response with read_at, got %+v", readResp.Data)
	}

	listAgainReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/announcements", nil)
	listAgainReq.AddCookie(sessionCookie)
	listAgainRec := httptest.NewRecorder()
	handler.ServeHTTP(listAgainRec, listAgainReq)
	if listAgainRec.Code != http.StatusOK {
		t.Fatalf("expected list after read 200, got %d body=%s", listAgainRec.Code, listAgainRec.Body.String())
	}
	var listAgainResp apiopenapi.UserAnnouncementListResponse
	if err := json.NewDecoder(listAgainRec.Body).Decode(&listAgainResp); err != nil {
		t.Fatalf("decode list after read response: %v", err)
	}
	if listAgainResp.Unread != 0 || len(listAgainResp.Data) != 1 || !listAgainResp.Data[0].Read {
		t.Fatalf("expected read state in list, got %+v", listAgainResp)
	}

	hiddenReadReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/announcements/"+string(hiddenAnnouncement.Data.Id)+"/read", nil)
	hiddenReadReq.AddCookie(sessionCookie)
	hiddenReadReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	hiddenReadRec := httptest.NewRecorder()
	handler.ServeHTTP(hiddenReadRec, hiddenReadReq)
	if hiddenReadRec.Code != http.StatusNotFound {
		t.Fatalf("expected hidden announcement read 404, got %d body=%s", hiddenReadRec.Code, hiddenReadRec.Body.String())
	}
}

func mustCreateAdminAnnouncement(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.AnnouncementResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected announcement create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AnnouncementResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode announcement response: %v", err)
	}
	return resp
}
