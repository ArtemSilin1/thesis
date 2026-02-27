package data

import "errors"

var (
	ErrURLNotFound = errors.New("not found")
	ErrURLExists   = errors.New("not exists")
)
