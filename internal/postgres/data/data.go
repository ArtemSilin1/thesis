package data

import "errors"

var (
	ErrNotFound  = errors.New("not found")
	ErrNotExists = errors.New("not exists")
	ErrExists    = errors.New("already exists")
)
