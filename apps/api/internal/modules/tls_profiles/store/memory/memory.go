package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
)

// Store is an in-memory implementation of the TLS fingerprint profile store.
type Store struct {
	mu       sync.Mutex
	profiles map[int]contract.Profile
	seq      int
	clock    func() time.Time
}

func New() *Store {
	return &Store{profiles: make(map[int]contract.Profile), clock: time.Now}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) CreateProfile(ctx context.Context, input contract.CreateProfile) (contract.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, profile := range s.profiles {
		if profile.Name == input.Name {
			return contract.Profile{}, contract.ErrDuplicateName
		}
	}
	s.seq++
	now := s.now()
	profile := contract.Profile{
		ID:                s.seq,
		Name:              input.Name,
		TLSTemplate:       input.TLSTemplate,
		HTTPVersionPolicy: input.HTTPVersionPolicy,
		UserAgent:         input.UserAgent,
		ExtraHeaders:      cloneHeaders(input.ExtraHeaders),
		Enabled:           input.Enabled,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.profiles[profile.ID] = profile
	return profile, nil
}

func (s *Store) UpdateProfile(ctx context.Context, id int, input contract.UpdateProfile) (contract.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[id]
	if !ok {
		return contract.Profile{}, contract.ErrNotFound
	}
	if input.Name != nil {
		for otherID, other := range s.profiles {
			if otherID != id && other.Name == *input.Name {
				return contract.Profile{}, contract.ErrDuplicateName
			}
		}
		profile.Name = *input.Name
	}
	if input.TLSTemplate != nil {
		profile.TLSTemplate = *input.TLSTemplate
	}
	if input.HTTPVersionPolicy != nil {
		profile.HTTPVersionPolicy = *input.HTTPVersionPolicy
	}
	if input.UserAgent != nil {
		profile.UserAgent = *input.UserAgent
	}
	if input.ExtraHeaders != nil {
		profile.ExtraHeaders = cloneHeaders(*input.ExtraHeaders)
	}
	if input.Enabled != nil {
		profile.Enabled = *input.Enabled
	}
	profile.UpdatedAt = s.now()
	s.profiles[id] = profile
	return profile, nil
}

func (s *Store) DeleteProfile(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.profiles[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.profiles, id)
	return nil
}

func (s *Store) ListProfiles(ctx context.Context) ([]contract.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Profile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		out = append(out, profile)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
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
