package main

import (
	"context"

	"dagger/remote-backend/internal/dagger"
)

type Local struct{}

func (m *RemoteBackend) Local() *Local {
	return &Local{}
}

func (b *Local) Import(ctx context.Context, key string) *dagger.Directory {
	return dag.Directory()
}

func (b *Local) Export(ctx context.Context, key string, dir *dagger.Directory) error {
	return nil
}
