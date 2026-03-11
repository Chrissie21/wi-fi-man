package service

import "errors"

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrNotFound         = errors.New("not found")
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrTokenRevoked     = errors.New("token revoked")
	ErrTokenConsumed    = errors.New("token consumed")
	ErrTokenDeviceBound = errors.New("token bound to a different device")
	ErrPlanInactive     = errors.New("plan inactive")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrUnauthorizedRole = errors.New("role unauthorized")
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionNotActive = errors.New("session not active")
)
