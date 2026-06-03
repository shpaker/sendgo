package domain

import "errors"

// Domain errors shared between use cases and adapters. Adapters map their
// native codes (S3 404, fs.ErrNotExist, redis.Nil) onto ErrNotFound, and the
// HTTP layer turns them into response codes (404, 401, 413, ...).
var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrBadRequest       = errors.New("bad request")
	ErrLimitReached     = errors.New("download limit reached")
	ErrPayloadTooLarge  = errors.New("payload too large")
	ErrInvalidShareID   = errors.New("invalid share id")
	ErrInvalidAuth      = errors.New("invalid authorization")
	ErrStorageUnhealthy = errors.New("storage unhealthy")
)
