package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dagger/example/internal/dagger"
)

type Example struct {
	Cache    *dagger.RemoteCache // +private
	CacheKey string              // +private
}

func New(registry string, repo string, cacheKey string) Example {
	backend := dag.RemoteBackend().Registry(registry, strings.ToLower(repo))
	cache := dag.RemoteCache(backend.AsRemoteCacheBackend())

	return Example{
		Cache:    cache,
		CacheKey: cacheKey,
	}
}

// +cache="session"
func (m Example) PrimeCache(
	ctx context.Context,
	msg string, // +default="Hello, world!"
) error {
	_, err := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.Mount("/example", m.CacheKey).AsDirectory).
		WithExec([]string{"sh", "-c", fmt.Sprintf("echo '%s' > /example/foo", msg)}).
		With(m.Cache.Export).
		Sync(ctx)
	return err
}

// +cache="session"
func (m Example) CheckCache(ctx context.Context) (string, error) {
	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.Mount("/example", m.CacheKey).AsDirectory).
		WithExec([]string{"sh", "-c", "cat /example/foo || echo 'CACHE MISS'"})

	return ctr.Stdout(ctx)
}
