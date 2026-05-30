package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	avatarSettingsKeyPrefix = "users.avatar:v1:user"
	avatarVersion           = "v1"
	avatarContentTypePNG    = "image/png"

	// MaxAvatarUploadBytes is the maximum accepted raw avatar upload size.
	MaxAvatarUploadBytes = 1 << 20
	// MaxAvatarStoredBytes is the maximum normalized PNG object size.
	MaxAvatarStoredBytes = 1 << 20
	// MaxAvatarDimension is the maximum accepted image width or height.
	MaxAvatarDimension = 1024
)

var (
	// ErrAvatarNotFound indicates that a user has no stored avatar.
	ErrAvatarNotFound = errors.New("user avatar not found")
	// ErrAvatarTooLarge indicates that either the upload or normalized PNG exceeded limits.
	ErrAvatarTooLarge = errors.New("user avatar too large")
)

// AvatarStore persists small user-owned avatar objects.
type AvatarStore interface {
	Get(ctx context.Context, key string) (map[string]any, bool, error)
	Set(ctx context.Context, key string, value map[string]any, updatedBy *int) error
}

// Avatar describes a normalized user avatar stored by SRapi.
type Avatar struct {
	UserID      int
	Content     []byte
	ContentType string
	ByteSize    int
	SHA256      string
	Width       int
	Height      int
	UpdatedAt   time.Time
}

// AvatarService validates and stores current-user avatars.
type AvatarService struct {
	store AvatarStore
	now   func() time.Time
}

func NewAvatarService(store AvatarStore, clock Clock) (*AvatarService, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	now := func() time.Time { return time.Now().UTC() }
	if clock != nil {
		now = func() time.Time { return clock.Now().UTC() }
	}
	return &AvatarService{store: store, now: now}, nil
}

func (s *AvatarService) Get(ctx context.Context, userID int) (Avatar, error) {
	if userID <= 0 {
		return Avatar{}, ErrInvalidInput
	}
	raw, found, err := s.store.Get(ctx, avatarSettingsKey(userID))
	if err != nil {
		return Avatar{}, err
	}
	if !found {
		return Avatar{}, ErrAvatarNotFound
	}
	avatar, err := avatarFromMap(userID, raw)
	if err != nil {
		return Avatar{}, err
	}
	return avatar, nil
}

func (s *AvatarService) Upsert(ctx context.Context, userID int, input io.Reader, updatedBy *int) (Avatar, error) {
	if userID <= 0 || input == nil {
		return Avatar{}, ErrInvalidInput
	}
	raw, err := io.ReadAll(io.LimitReader(input, MaxAvatarUploadBytes+1))
	if err != nil {
		return Avatar{}, err
	}
	avatar, err := normalizeAvatarBytes(userID, raw, s.now())
	if err != nil {
		return Avatar{}, err
	}
	if err := s.store.Set(ctx, avatarSettingsKey(userID), avatar.toMap(), updatedBy); err != nil {
		return Avatar{}, err
	}
	return avatar, nil
}

func (s *AvatarService) Delete(ctx context.Context, userID int, updatedBy *int) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	return s.store.Set(ctx, avatarSettingsKey(userID), map[string]any{
		"version": avatarVersion,
		"user_id": userID,
		"deleted": true,
	}, updatedBy)
}

func (s *AvatarService) DecorateUser(ctx context.Context, user contract.User, avatarURL string) contract.User {
	if user.ID <= 0 {
		return user
	}
	avatar, err := s.Get(ctx, user.ID)
	if err != nil {
		return user
	}
	user.AvatarURL = strings.TrimSpace(avatarURL)
	user.AvatarMIME = avatar.ContentType
	user.AvatarByteSize = avatar.ByteSize
	user.AvatarSHA256 = avatar.SHA256
	updatedAt := avatar.UpdatedAt
	user.AvatarUpdatedAt = &updatedAt
	return user
}

func normalizeAvatarBytes(userID int, raw []byte, updatedAt time.Time) (Avatar, error) {
	if userID <= 0 || len(raw) == 0 {
		return Avatar{}, ErrInvalidInput
	}
	if len(raw) > MaxAvatarUploadBytes {
		return Avatar{}, ErrAvatarTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Avatar{}, ErrInvalidInput
	}
	if format != "png" && format != "jpeg" {
		return Avatar{}, ErrInvalidInput
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxAvatarDimension || cfg.Height > MaxAvatarDimension {
		return Avatar{}, ErrInvalidInput
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != format {
		return Avatar{}, ErrInvalidInput
	}
	bounds := img.Bounds()
	if bounds.Empty() || bounds.Dx() > MaxAvatarDimension || bounds.Dy() > MaxAvatarDimension {
		return Avatar{}, ErrInvalidInput
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return Avatar{}, err
	}
	content := encoded.Bytes()
	if len(content) > MaxAvatarStoredBytes {
		return Avatar{}, ErrAvatarTooLarge
	}
	sum := sha256.Sum256(content)
	return Avatar{
		UserID:      userID,
		Content:     append([]byte(nil), content...),
		ContentType: avatarContentTypePNG,
		ByteSize:    len(content),
		SHA256:      hex.EncodeToString(sum[:]),
		Width:       bounds.Dx(),
		Height:      bounds.Dy(),
		UpdatedAt:   updatedAt.UTC(),
	}, nil
}

func avatarFromMap(userID int, raw map[string]any) (Avatar, error) {
	if mapBool(raw["deleted"]) {
		return Avatar{}, ErrAvatarNotFound
	}
	if storedUserID := mapInt(raw["user_id"]); storedUserID > 0 && storedUserID != userID {
		return Avatar{}, ErrInvalidInput
	}
	contentType := strings.TrimSpace(fmt.Sprint(raw["content_type"]))
	if contentType != avatarContentTypePNG {
		return Avatar{}, ErrInvalidInput
	}
	encoded := strings.TrimSpace(fmt.Sprint(raw["content_base64"]))
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(content) == 0 || len(content) > MaxAvatarStoredBytes {
		return Avatar{}, ErrInvalidInput
	}
	sum := sha256.Sum256(content)
	shaHex := hex.EncodeToString(sum[:])
	storedSHA := strings.TrimSpace(fmt.Sprint(raw["sha256"]))
	if storedSHA == "" {
		storedSHA = shaHex
	}
	if storedSHA != shaHex {
		return Avatar{}, ErrInvalidInput
	}
	updatedAt := mapTime(raw["updated_at"])
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return Avatar{
		UserID:      userID,
		Content:     content,
		ContentType: contentType,
		ByteSize:    len(content),
		SHA256:      shaHex,
		Width:       mapInt(raw["width"]),
		Height:      mapInt(raw["height"]),
		UpdatedAt:   updatedAt.UTC(),
	}, nil
}

func (a Avatar) toMap() map[string]any {
	return map[string]any{
		"version":        avatarVersion,
		"user_id":        a.UserID,
		"content_type":   a.ContentType,
		"content_base64": base64.StdEncoding.EncodeToString(a.Content),
		"byte_size":      a.ByteSize,
		"sha256":         a.SHA256,
		"width":          a.Width,
		"height":         a.Height,
		"updated_at":     a.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func avatarSettingsKey(userID int) string {
	return avatarSettingsKeyPrefix + ":" + strconv.Itoa(userID)
}

func mapBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), "true")
	}
}

func mapInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		return parsed
	}
}

func mapTime(value any) time.Time {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
