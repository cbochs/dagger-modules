package main

import (
	"context"
	"strings"
	"time"
)

type RemoteCache struct {
	Cache    *Cache // +private
	CacheKey string // +private
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
) RemoteCache {
	cache := NewCache().WithRemote(registry, strings.ToLower(repo))
	if skipIfExists {
		cache = cache.WithSkipIfExists()
	}

	return RemoteCache{
		Cache:    cache,
		CacheKey: cacheKey,
	}
}

func (m RemoteCache) PrimeCache(ctx context.Context) error {
	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.MountedVolume("/example", m.CacheKey)).
		WithExec([]string{"sh", "-c", "echo 'Hello, world' > /example/foo"})

	ctr, err := m.Cache.Sync(ctx, ctr)

	return err
}

func (m RemoteCache) CheckCache(ctx context.Context) (string, error) {
	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(m.Cache.MountedVolume("/example", m.CacheKey)).
		WithExec([]string{"sh", "-c", "cat /example/foo || echo 'CACHE MISS'"})

	err := m.Cache.Download(ctx)
	if err != nil {
		return "", err
	}

	return ctr.Stdout(ctx)
}
