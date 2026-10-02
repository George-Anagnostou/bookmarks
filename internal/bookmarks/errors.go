package bookmarks

import "errors"

var (
	ErrEmptyURL          = errors.New("url is required")
	ErrUnsupportedScheme = errors.New("url must use http or https")
	ErrMissingHost       = errors.New("url host is required")
	ErrURLUserInfo       = errors.New("url must not include credentials")
	ErrNotFound          = errors.New("bookmark not found")
	ErrDuplicateURL      = errors.New("bookmark url already exists")
	ErrNoUpdateFields    = errors.New("bookmark edit must update at least one field")
)
