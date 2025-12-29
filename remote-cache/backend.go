package main

import (
	"context"

	"dagger/remote-cache/internal/dagger"
)

type Backend interface {
	DaggerObject

	Import(ctx context.Context, key string) *dagger.Directory
	Export(ctx context.Context, key string, dir *dagger.Directory) error
}
