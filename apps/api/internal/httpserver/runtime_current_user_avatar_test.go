package httpserver

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestCurrentUserAvatarUploadServeAndDelete(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	avatarPNG := tinyAvatarPNG(t)

	missingCSRFReq := currentUserAvatarUploadRequest(t, avatarPNG)
	missingCSRFReq.AddCookie(sessionCookie)
	missingCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRec, missingCSRFReq)
	if missingCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing csrf 403, got %d body=%s", missingCSRFRec.Code, missingCSRFRec.Body.String())
	}

	uploadReq := currentUserAvatarUploadRequest(t, avatarPNG)
	uploadReq.AddCookie(sessionCookie)
	uploadReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("expected avatar upload 200, got %d body=%s", uploadRec.Code, uploadRec.Body.String())
	}
	var uploadResp apiopenapi.UserAvatarResponse
	if err := json.NewDecoder(uploadRec.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode avatar response: %v", err)
	}
	if uploadResp.Data.ContentType != apiopenapi.UserAvatarContentType("image/png") || uploadResp.Data.ByteSize <= 0 || uploadResp.Data.Sha256 == "" || uploadResp.Data.Width != 2 || uploadResp.Data.Height != 2 {
		t.Fatalf("unexpected avatar response: %+v", uploadResp.Data)
	}
	if !strings.HasPrefix(uploadResp.Data.Url, "/api/v1/users/"+string(loginResp.Data.User.Id)+"/avatar?v=") {
		t.Fatalf("expected versioned avatar url, got %q", uploadResp.Data.Url)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d body=%s", meRec.Code, meRec.Body.String())
	}
	var meResp apiopenapi.UserResponse
	if err := json.NewDecoder(meRec.Body).Decode(&meResp); err != nil {
		t.Fatalf("decode current user response: %v", err)
	}
	if meResp.Data.AvatarUrl == nil || !strings.HasPrefix(*meResp.Data.AvatarUrl, "/api/v1/users/"+string(loginResp.Data.User.Id)+"/avatar") {
		t.Fatalf("expected current user avatar url, got %+v", meResp.Data.AvatarUrl)
	}
	if meResp.Data.AvatarSha256 == nil || *meResp.Data.AvatarSha256 != uploadResp.Data.Sha256 {
		t.Fatalf("expected current user avatar sha, got %+v", meResp.Data.AvatarSha256)
	}

	imageReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+string(loginResp.Data.User.Id)+"/avatar", nil)
	imageReq.AddCookie(sessionCookie)
	imageRec := httptest.NewRecorder()
	handler.ServeHTTP(imageRec, imageReq)
	if imageRec.Code != http.StatusOK {
		t.Fatalf("expected avatar image 200, got %d body=%s", imageRec.Code, imageRec.Body.String())
	}
	if got := imageRec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png content type, got %q", got)
	}
	if got := imageRec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if imageRec.Body.Len() != uploadResp.Data.ByteSize {
		t.Fatalf("expected served byte size %d, got %d", uploadResp.Data.ByteSize, imageRec.Body.Len())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/me/avatar", nil)
	deleteReq.AddCookie(sessionCookie)
	deleteReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected avatar delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	imageAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+string(loginResp.Data.User.Id)+"/avatar", nil)
	imageAfterDeleteReq.AddCookie(sessionCookie)
	imageAfterDeleteRec := httptest.NewRecorder()
	handler.ServeHTTP(imageAfterDeleteRec, imageAfterDeleteReq)
	if imageAfterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("expected avatar image 404 after delete, got %d body=%s", imageAfterDeleteRec.Code, imageAfterDeleteRec.Body.String())
	}
}

func currentUserAvatarUploadRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatalf("create avatar form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write avatar payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/me/avatar", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func tinyAvatarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 32, G: 96, B: 220, A: 255})
	img.Set(1, 0, color.RGBA{R: 220, G: 96, B: 32, A: 255})
	img.Set(0, 1, color.RGBA{R: 32, G: 180, B: 96, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
