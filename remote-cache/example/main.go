package main

import (
	"context"
	"time"

	"dagger/example/internal/dagger"
)

type Example struct {
	Cache    *dagger.RemoteCache // +private
	CacheKey string              // +private
}

func New(
	// +default="ttl.sh"
	registry string, // +optional
	// +default="8668e22b-07c9-4b90-b6de-e15738864818"
	repo string, // +optional
	// +optional
	skipIfExists bool, // +optional
	// +default="2h"
	cacheKey string, // +optional
) Example {
	cache := dag.RemoteCache(registry, repo)
	// if skipIfExists {
	// 	cache = cache.WithSkipIfExists()
	// }

	return Example{
		Cache:    cache,
		CacheKey: cacheKey,
	}
}

// +cache="session"
func (m Example) PrimeCache(ctx context.Context) error {
	_, err := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.CacheVolume("/example", m.CacheKey).Mount).
		WithMountedCache("/todo", dag.CacheVolume("asdfasdf")).
		WithExec([]string{"sh", "-c", "echo 'Hello, world' > /example/foo"}).
		Sync(ctx)
	if err != nil {
		return err
	}

	return m.Cache.Export(ctx)
}

// +cache="session"
func (m Example) CheckCache(ctx context.Context) (string, error) {
	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.CacheVolume("/example", m.CacheKey).Mount).
		WithExec([]string{"sh", "-c", "cat /example/foo || echo 'CACHE MISS'"})

	return ctr.Stdout(ctx)
}
