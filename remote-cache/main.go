package main

import (
	"context"
	"fmt"
	"time"
)

const CacheKey = "8668e22b-07c9-4b90-b6de-e15738864818"

func TtlKeyFormatter(registry, repo, key, arch string) string {
	return fmt.Sprintf("%s/%s:2h", registry, CacheKey)
}

type RemoteCache struct{}

func (m RemoteCache) PrimeCache(ctx context.Context) error {
	cache := NewCache().
		WithRemote("ttl.sh", "").
		WithSkipIfExists().
		WithKeyFormatter(TtlKeyFormatter)

	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(cache.MountedVolume("/example", "example")).
		WithExec([]string{"sh", "-c", "echo 'Hello, world' > /example/foo"})

	ctr, err := cache.Sync(ctx, ctr)

	return err
}

func (m RemoteCache) CheckCache(ctx context.Context) (string, error) {
	cache := NewCache().
		WithRemote("ttl.sh", "").
		WithSkipIfExists().
		WithKeyFormatter(TtlKeyFormatter)

	ctr := dag.Container().
		From("alpine").
		WithEnvVariable("CACHEBUST", time.Now().String()).
		With(cache.MountedVolume("/example", "example")).
		WithExec([]string{"sh", "-c", "cat /example/foo || echo 'CACHE MISS'"})

	err := cache.Download(ctx)
	if err != nil {
		return "", err
	}

	return ctr.Stdout(ctx)
}
