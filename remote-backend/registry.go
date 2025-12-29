package main

import (
	"context"
	"fmt"

	"dagger/remote-backend/internal/dagger"
)

// OCI registry backend
type Registry struct {
	Registry string         // +private
	Repo     string         // +private
	Username string         // +private
	Secret   *dagger.Secret // +private
}

func (m *RemoteBackend) Registry(
	// Image registry address
	registry string,
	// Image repo
	repo string,
	// Image registry username
	// +optional
	username string,
	// Image registry password
	// +optional
	password *dagger.Secret,
) *Registry {
	return &Registry{
		Registry: registry,
		Repo:     repo,
		Username: username,
		Secret:   password,
	}
}

// +cache="session"
func (b *Registry) Import(ctx context.Context, key string) *dagger.Directory {
	imageAddr := cacheImageAddr(b.Registry, b.Repo, key)

	ctr := dag.Container()
	if b.Username != "" && b.Secret != nil {
		ctr = ctr.WithRegistryAuth(b.Registry, b.Username, b.Secret)
	}

	ctr, err := ctr.From(imageAddr).Sync(ctx)
	if err != nil {
		return dag.Directory()
	}

	return ctr.Directory("")
}

// +cache="session"
func (b *Registry) Export(ctx context.Context, key string, dir *dagger.Directory) error {
	imageAddr := cacheImageAddr(b.Registry, b.Repo, key)
	_, err := dag.Container().WithDirectory("", dir).Publish(ctx, imageAddr)
	return err
}

func cacheImageAddr(registry string, repo string, key string) string {
	return fmt.Sprintf("%s/%s:%s", registry, repo, key)
}
