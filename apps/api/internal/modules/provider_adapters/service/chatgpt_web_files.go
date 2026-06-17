// ChatGPT-web file upload + multimodal asset_pointer construction — ported
// from chatgpt2api services/openai_backend_api.py:_upload_image (~L851-L908)
// and the multimodal_text payload construction at _start_image_generation.
//
// The upload sequence verbatim from chatgpt2api:
//   1. POST /backend-api/files  body: {file_name, file_size, use_case:
//        "multimodal", width, height}  →  {file_id, upload_url}
//   2. PUT  <upload_url>          headers: Content-Type=<mime>,
//        x-ms-blob-type: BlockBlob, x-ms-version: 2020-04-08, Origin,
//        Referer, User-Agent, Accept, Accept-Language
//        body: raw bytes
//   3. POST /backend-api/files/{file_id}/uploaded   body: "{}"
//   4. Reference the file as a part:
//        {"content_type":"image_asset_pointer",
//         "asset_pointer":"file-service://<file_id>",
//         "width":W,"height":H,"size_bytes":N}
//
// Width/height extraction uses image/png and image/jpeg from the stdlib
// (Pillow equivalent). Non-PNG/JPEG images upload with their declared width
// and height, falling back to 0 when the caller doesn't supply them — same
// as chatgpt2api's behaviour when Pillow cannot decode.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// ChatGPTWebFileAsset is the result of an upload — what the chatgpt_web
// payload builder needs to construct a multimodal_text part.
type ChatGPTWebFileAsset struct {
	FileID       string
	FileName     string
	MimeType     string
	Width        int
	Height       int
	SizeBytes    int
	AssetPointer string // "file-service://<file_id>"
}

// ChatGPTWebUploadSession carries the per-request identity needed to upload.
// We accept the bits we need rather than a heavy contract.ConversationRequest
// so the helper is testable independently.
type ChatGPTWebUploadSession struct {
	Account   reverseproxycontract.AccountRuntime
	BaseURL   string
	Origin    string
	UserAgent string
	Headers   http.Header
}

// chatGPTWebFileUploader uploads a single image to ChatGPT's
// /backend-api/files endpoint via the reverse-proxy runtime, exactly as
// chatgpt2api does it.
type chatGPTWebFileUploader struct {
	rp reverseproxycontract.Runtime
}

func newChatGPTWebFileUploader(rp reverseproxycontract.Runtime) *chatGPTWebFileUploader {
	return &chatGPTWebFileUploader{rp: rp}
}

// uploadImage performs the three-step chatgpt2api upload sequence and
// returns a ChatGPTWebFileAsset whose AssetPointer is ready to drop into
// a multimodal_text "parts" entry.
func (u *chatGPTWebFileUploader) uploadImage(ctx context.Context, sess ChatGPTWebUploadSession, body []byte, mimeType, fileName string) (*ChatGPTWebFileAsset, error) {
	if u == nil || u.rp == nil {
		return nil, errors.New("chatgpt web uploader: reverse proxy runtime unavailable")
	}
	if len(body) == 0 {
		return nil, errors.New("chatgpt web uploader: empty body")
	}
	mimeType = chatGPTWebNormaliseMimeType(mimeType, body)
	fileName = chatGPTWebDefaultFileName(fileName, mimeType)
	width, height := chatGPTWebImageDimensions(body)

	// Step 1: POST /backend-api/files  →  {file_id, upload_url}
	registerPath := "/backend-api/files"
	registerHeaders := chatGPTWebFileUploadJSONHeaders(sess, registerPath)
	registerBody, err := json.Marshal(map[string]any{
		"file_name": fileName,
		"file_size": len(body),
		"use_case":  "multimodal",
		"width":     width,
		"height":    height,
	})
	if err != nil {
		return nil, fmt.Errorf("chatgpt web uploader: marshal register: %w", err)
	}
	registerResp, err := u.rp.Do(ctx, reverseproxycontract.Request{
		Account: sess.Account,
		Method:  http.MethodPost,
		URL:     strings.TrimRight(sess.BaseURL, "/") + registerPath,
		Headers: registerHeaders,
		Body:    registerBody,
	})
	if err != nil {
		return nil, fmt.Errorf("chatgpt web uploader: register: %w", err)
	}
	if registerResp.StatusCode < 200 || registerResp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatgpt web uploader: register status %d: %s", registerResp.StatusCode, truncateForError(registerResp.Body))
	}
	var registered struct {
		FileID    string `json:"file_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(registerResp.Body, &registered); err != nil {
		return nil, fmt.Errorf("chatgpt web uploader: decode register: %w", err)
	}
	if registered.FileID == "" || registered.UploadURL == "" {
		return nil, errors.New("chatgpt web uploader: register response missing file_id or upload_url")
	}

	// Step 2: PUT upload_url with the binary body.
	if err := u.putUpload(ctx, sess, registered.UploadURL, mimeType, body); err != nil {
		return nil, err
	}

	// Step 3: POST /backend-api/files/{file_id}/uploaded
	finalisePath := "/backend-api/files/" + registered.FileID + "/uploaded"
	finaliseHeaders := chatGPTWebFileUploadJSONHeaders(sess, finalisePath)
	finaliseResp, err := u.rp.Do(ctx, reverseproxycontract.Request{
		Account: sess.Account,
		Method:  http.MethodPost,
		URL:     strings.TrimRight(sess.BaseURL, "/") + finalisePath,
		Headers: finaliseHeaders,
		Body:    []byte("{}"),
	})
	if err != nil {
		return nil, fmt.Errorf("chatgpt web uploader: finalise: %w", err)
	}
	if finaliseResp.StatusCode < 200 || finaliseResp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatgpt web uploader: finalise status %d: %s", finaliseResp.StatusCode, truncateForError(finaliseResp.Body))
	}

	return &ChatGPTWebFileAsset{
		FileID:       registered.FileID,
		FileName:     fileName,
		MimeType:     mimeType,
		Width:        width,
		Height:       height,
		SizeBytes:    len(body),
		AssetPointer: "file-service://" + registered.FileID,
	}, nil
}

// putUpload PUTs the raw bytes to the cloud blob URL. This bypasses the
// reverse-proxy runtime because the upload URL is a SAS-signed Azure /
// Cloudflare R2 endpoint that doesn't carry the OpenAI session — calling
// it via the per-account egress would inject the wrong Authorization.
func (u *chatGPTWebFileUploader) putUpload(ctx context.Context, sess ChatGPTWebUploadSession, uploadURL, mimeType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("chatgpt web uploader: build upload request: %w", err)
	}
	// Matches the chatgpt2api header set for Azure blob PUT.
	origin := strings.TrimRight(sess.Origin, "/")
	if origin == "" {
		origin = strings.TrimRight(sess.BaseURL, "/")
	}
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("x-ms-version", "2020-04-08")
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/")
	}
	if ua := strings.TrimSpace(sess.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")

	client := &http.Client{}
	if managed, ok, err := u.rp.ManagedEgressClient(sess.Account); err == nil && ok && managed != nil {
		client = managed
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("chatgpt web uploader: upload put: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("chatgpt web uploader: upload put status %d: %s", resp.StatusCode, truncateForError(raw))
	}
	return nil
}

func chatGPTWebFileUploadJSONHeaders(sess ChatGPTWebUploadSession, path string) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json"},
	}
	// Inherit shared base headers (Origin, Referer, UA, OAI-* identifiers) so
	// the upload endpoints receive the same client fingerprint as the
	// conversation request that triggered them.
	for k, vv := range sess.Headers {
		if _, exists := headers[k]; exists {
			continue
		}
		headers[k] = append([]string(nil), vv...)
	}
	headers.Set("X-OpenAI-Target-Path", path)
	headers.Set("X-OpenAI-Target-Route", path)
	return headers
}

func chatGPTWebImageDimensions(body []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func chatGPTWebNormaliseMimeType(mime string, body []byte) string {
	mime = strings.TrimSpace(strings.ToLower(mime))
	if mime != "" {
		return mime
	}
	if sniff := http.DetectContentType(body); sniff != "" {
		return sniff
	}
	return "application/octet-stream"
}

func chatGPTWebDefaultFileName(name, mime string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	switch mime {
	case "image/png":
		return "image.png"
	case "image/jpeg", "image/jpg":
		return "image.jpg"
	case "image/gif":
		return "image.gif"
	case "image/webp":
		return "image.webp"
	default:
		return "upload.bin"
	}
}

func truncateForError(body []byte) string {
	const max = 512
	out := strings.TrimSpace(string(body))
	if len(out) <= max {
		return out
	}
	return out[:max] + "..."
}

// chatGPTWebAssetPointerPart builds the multimodal "parts" entry chatgpt2api
// emits for an uploaded reference asset.
func chatGPTWebAssetPointerPart(asset *ChatGPTWebFileAsset) map[string]any {
	if asset == nil {
		return nil
	}
	return map[string]any{
		"content_type":  "image_asset_pointer",
		"asset_pointer": asset.AssetPointer,
		"width":         asset.Width,
		"height":        asset.Height,
		"size_bytes":    asset.SizeBytes,
	}
}

// chatGPTWebAttachmentEntry builds the metadata.attachments entry that
// chatgpt2api appends alongside the parts.
func chatGPTWebAttachmentEntry(asset *ChatGPTWebFileAsset) map[string]any {
	if asset == nil {
		return nil
	}
	return map[string]any{
		"id":       asset.FileID,
		"mimeType": asset.MimeType,
		"name":     asset.FileName,
		"size":     asset.SizeBytes,
		"width":    asset.Width,
		"height":   asset.Height,
	}
}
