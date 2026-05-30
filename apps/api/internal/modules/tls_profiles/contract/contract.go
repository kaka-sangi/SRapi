package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a profile does not exist.
var ErrNotFound = errors.New("tls fingerprint profile not found")

// ErrDuplicateName is returned when a profile name already exists.
var ErrDuplicateName = errors.New("tls fingerprint profile name already exists")

// Profile is one named egress fingerprint profile.
type Profile struct {
	ID                int
	Name              string
	TLSTemplate       string
	HTTPVersionPolicy string
	UserAgent         string
	ExtraHeaders      map[string]string
	Enabled           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateProfile struct {
	Name              string
	TLSTemplate       string
	HTTPVersionPolicy string
	UserAgent         string
	ExtraHeaders      map[string]string
	Enabled           bool
}

type UpdateProfile struct {
	Name              *string
	TLSTemplate       *string
	HTTPVersionPolicy *string
	UserAgent         *string
	ExtraHeaders      *map[string]string
	Enabled           *bool
}

// Store persists named TLS fingerprint profiles.
type Store interface {
	CreateProfile(ctx context.Context, input CreateProfile) (Profile, error)
	UpdateProfile(ctx context.Context, id int, input UpdateProfile) (Profile, error)
	DeleteProfile(ctx context.Context, id int) error
	ListProfiles(ctx context.Context) ([]Profile, error)
}
