package tlsprofiles

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entprofile "github.com/srapi/srapi/apps/api/ent/tlsfingerprintprofile"
	"github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
)

var ErrInvalidStore = errors.New("invalid tls fingerprint profile ent store")

// Store is the Ent-backed implementation of the TLS fingerprint profile store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateProfile(ctx context.Context, input contract.CreateProfile) (contract.Profile, error) {
	now := time.Now().UTC()
	row, err := s.client.TLSFingerprintProfile.Create().
		SetName(input.Name).
		SetTLSTemplate(input.TLSTemplate).
		SetHTTPVersionPolicy(input.HTTPVersionPolicy).
		SetUserAgent(input.UserAgent).
		SetExtraHeaders(cloneHeaders(input.ExtraHeaders)).
		SetEnabled(input.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.Profile{}, contract.ErrDuplicateName
		}
		return contract.Profile{}, err
	}
	return toProfile(row), nil
}

func (s *Store) UpdateProfile(ctx context.Context, id int, input contract.UpdateProfile) (contract.Profile, error) {
	if id <= 0 {
		return contract.Profile{}, ErrInvalidStore
	}
	update := s.client.TLSFingerprintProfile.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.TLSTemplate != nil {
		update.SetTLSTemplate(*input.TLSTemplate)
	}
	if input.HTTPVersionPolicy != nil {
		update.SetHTTPVersionPolicy(*input.HTTPVersionPolicy)
	}
	if input.UserAgent != nil {
		update.SetUserAgent(*input.UserAgent)
	}
	if input.ExtraHeaders != nil {
		update.SetExtraHeaders(cloneHeaders(*input.ExtraHeaders))
	}
	if input.Enabled != nil {
		update.SetEnabled(*input.Enabled)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Profile{}, contract.ErrNotFound
		}
		if ent.IsConstraintError(err) {
			return contract.Profile{}, contract.ErrDuplicateName
		}
		return contract.Profile{}, err
	}
	return toProfile(row), nil
}

func (s *Store) DeleteProfile(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if err := s.client.TLSFingerprintProfile.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) ListProfiles(ctx context.Context) ([]contract.Profile, error) {
	rows, err := s.client.TLSFingerprintProfile.Query().
		Order(ent.Asc(entprofile.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProfile(row))
	}
	return out, nil
}

func toProfile(row *ent.TLSFingerprintProfile) contract.Profile {
	return contract.Profile{
		ID:                row.ID,
		Name:              row.Name,
		TLSTemplate:       row.TLSTemplate,
		HTTPVersionPolicy: row.HTTPVersionPolicy,
		UserAgent:         row.UserAgent,
		ExtraHeaders:      cloneHeaders(row.ExtraHeaders),
		Enabled:           row.Enabled,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}
