package blob

import "context"

type Pinger interface {
	Ping(ctx context.Context) error
}

type Store interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
	Get(ctx context.Context, key string) ([]byte, string, error)
	Delete(ctx context.Context, key string) error
}
