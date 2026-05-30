package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a definition or value does not exist.
var ErrNotFound = errors.New("user attribute not found")

// ErrDuplicateKey is returned when a definition key already exists.
var ErrDuplicateKey = errors.New("user attribute key already exists")

type DataType string

const (
	DataTypeString  DataType = "string"
	DataTypeNumber  DataType = "number"
	DataTypeBoolean DataType = "boolean"
	DataTypeSelect  DataType = "select"
)

// Definition is one operator-defined custom user attribute.
type Definition struct {
	ID           int
	Key          string
	Name         string
	DataType     DataType
	Options      []string
	Required     bool
	DisplayOrder int
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Value is one user's value for a definition.
type Value struct {
	ID           int
	UserID       int
	DefinitionID int
	Value        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateDefinition struct {
	Key          string
	Name         string
	DataType     DataType
	Options      []string
	Required     bool
	DisplayOrder int
	Enabled      bool
}

type UpdateDefinition struct {
	Name         *string
	DataType     *DataType
	Options      *[]string
	Required     *bool
	DisplayOrder *int
	Enabled      *bool
}

type SetValue struct {
	UserID       int
	DefinitionID int
	Value        string
}

// Store persists user attribute definitions and per-user values.
type Store interface {
	CreateDefinition(ctx context.Context, input CreateDefinition) (Definition, error)
	UpdateDefinition(ctx context.Context, id int, input UpdateDefinition) (Definition, error)
	DeleteDefinition(ctx context.Context, id int) error
	FindDefinitionByID(ctx context.Context, id int) (Definition, error)
	ListDefinitions(ctx context.Context) ([]Definition, error)
	UpsertValue(ctx context.Context, input SetValue) (Value, error)
	ListValuesByUser(ctx context.Context, userID int) ([]Value, error)
	DeleteValuesByUser(ctx context.Context, userID int) error
}
