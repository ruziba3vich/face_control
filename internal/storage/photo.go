package storage

import (
	"context"
	"mime/multipart"
)

// PhotoStore is the interface the rest of the app uses. Save returns an
// opaque key (e.g., S3 object key) that the caller persists; Get retrieves
// the bytes by that key.
type PhotoStore interface {
	Save(ctx context.Context, file multipart.File, header *multipart.FileHeader) (key string, err error)
	Get(ctx context.Context, key string) ([]byte, error)
}
