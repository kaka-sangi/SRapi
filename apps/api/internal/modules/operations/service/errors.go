package service

import (
	"errors"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

var (
	ErrInvalidInput = errors.New("invalid operations input")
	ErrNotFound     = contract.ErrNotFound
)
