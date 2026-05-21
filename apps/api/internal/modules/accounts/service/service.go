package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const credentialVersionV1 = "v1"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store     contract.Store
	masterKey []byte
	clock     Clock
}

func New(store contract.Store, masterKey string, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if len(masterKey) < 32 {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	derivedKey, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store, masterKey: derivedKey, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, req contract.CreateRequest) (contract.ProviderAccount, error) {
	name := strings.TrimSpace(req.Name)
	if req.ProviderID <= 0 || name == "" || req.RuntimeClass == "" {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	if len(req.Credential) == 0 {
		return contract.ProviderAccount{}, ErrCredentialMissing
	}
	credentialCiphertext, err := s.encryptCredential(req.Credential)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	status := contract.StatusActive
	if req.Status != nil {
		status = *req.Status
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	weight := float32(1.0)
	if req.Weight != nil {
		weight = *req.Weight
	}

	stored, err := s.store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           req.ProviderID,
		Name:                 name,
		RuntimeClass:         req.RuntimeClass,
		CredentialCiphertext: credentialCiphertext,
		CredentialVersion:    credentialVersionV1,
		Metadata:             cloneMap(req.Metadata),
		ProxyID:              req.ProxyID,
		Status:               status,
		Priority:             priority,
		Weight:               weight,
		UpstreamClient:       req.UpstreamClient,
	})
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return stored, nil
}

func (s *Service) List(ctx context.Context) ([]contract.ProviderAccount, error) {
	accounts, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, account)
	}
	return out, nil
}

func (s *Service) ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListGroupIDsByAccount(ctx, accountID)
}

func (s *Service) FindByID(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int, req contract.UpdateRequest) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.Name = name
	}
	if req.RuntimeClass != nil {
		if *req.RuntimeClass == "" {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.RuntimeClass = *req.RuntimeClass
	}
	if req.Credential != nil {
		if len(*req.Credential) == 0 {
			return contract.ProviderAccount{}, ErrCredentialMissing
		}
		ciphertext, err := s.encryptCredential(*req.Credential)
		if err != nil {
			return contract.ProviderAccount{}, err
		}
		account.CredentialCiphertext = ciphertext
		account.CredentialVersion = credentialVersionV1
	}
	if req.Metadata != nil {
		account.Metadata = cloneMap(*req.Metadata)
	}
	if req.ProxyID != nil {
		account.ProxyID = cloneString(*req.ProxyID)
	}
	if req.Status != nil {
		account.Status = *req.Status
	}
	if req.Priority != nil {
		account.Priority = *req.Priority
	}
	if req.Weight != nil {
		if *req.Weight < 0 {
			return contract.ProviderAccount{}, ErrInvalidInput
		}
		account.Weight = *req.Weight
	}
	if req.UpstreamClient != nil {
		account.UpstreamClient = cloneString(*req.UpstreamClient)
	}
	account.UpdatedAt = s.clock.Now()
	return s.store.Update(ctx, account)
}

func (s *Service) DecryptCredential(ctx context.Context, id int) (map[string]any, error) {
	if id <= 0 {
		return nil, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.decryptCredential(account.CredentialCiphertext)
}

func (s *Service) encryptCredential(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte(credentialVersionV1))
	return fmt.Sprintf("%s:%s:%s", credentialVersionV1, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Service) decryptCredential(ciphertext string) (map[string]any, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != credentialVersionV1 {
		return nil, ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(credentialVersionV1))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
