package main

import (
	"context"
	"fmt"

	"dagger/remote-cache/internal/dagger"
	"dagger/remote-cache/internal/telemetry"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// type Backend interface {
// 	Exists(ctx context.Context, key string) bool
// 	Import(ctx context.Context, key string) (*dagger.Directory, error)
// 	Export(ctx context.Context, key string, dir *dagger.Directory) error
//
// 	DaggerObject
// }

type Backend struct {
	// OCI registry backend
	Registry string         // +private
	Repo     string         // +private
	Username string         // +private
	Secret   *dagger.Secret // +private
}

func NewRegistry(registry string, repo string) *Backend {
	return &Backend{
		Registry: registry,
		Repo:     repo,
	}
}

// +cache="session"
func (b *Backend) Exists(ctx context.Context, key string) bool {
	imageAddr := cacheKeyAddr(b.Registry, b.Repo, key)

	ctx, span := Tracer().Start(ctx, fmt.Sprintf("Backend.exists(key: %q, image: %q)", key, imageAddr))
	span.SetAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.image", imageAddr),
	)
	defer telemetry.EndWithCause(span, nil)

	imageName := "docker://" + imageAddr

	_, parseSpan := Tracer().Start(ctx, "parse image address")
	imageRef, err := alltransports.ParseImageName(imageName)
	if err != nil {
		telemetry.EndWithCause(parseSpan, &err)
		return false
	}
	telemetry.EndWithCause(parseSpan, nil)

	_, fetchSpan := Tracer().Start(ctx, "fetch image metadata")
	sys := &types.SystemContext{}
	imageSrc, err := imageRef.NewImageSource(ctx, sys)
	if err != nil {
		telemetry.EndWithCause(fetchSpan, &err)
		return false
	}
	telemetry.EndWithCause(fetchSpan, nil)
	defer imageSrc.Close()

	return true
}

// +cache="session"
func (b *Backend) Import(ctx context.Context, key string) (*dagger.Directory, error) {
	ctx, span := Tracer().Start(
		ctx,
		fmt.Sprintf("Backend.import(key: %q)", key),
		trace.WithAttributes(attribute.String("cache.key", key)),
	)
	defer telemetry.EndWithCause(span, nil)

	if !b.Exists(ctx, key) {
		return dag.Directory(), nil
	}

	cacheAddr := cacheKeyAddr(b.Registry, b.Repo, key)
	_, pullSpan := Tracer().Start(
		ctx,
		fmt.Sprintf("pull cache image: %s", cacheAddr),
		trace.WithAttributes(attribute.String("cache.image", cacheAddr)),
	)

	dir, err := dag.Container().From(cacheAddr).Directory("").Sync(ctx)
	telemetry.EndWithCause(pullSpan, &err)

	return dir, err
}

// +cache="session"
func (b *Backend) Export(ctx context.Context, key string, dir *dagger.Directory) error {
	cacheAddr := cacheKeyAddr(b.Registry, b.Repo, key)

	ctx, span := Tracer().Start(
		ctx,
		fmt.Sprintf("Backend.export(key: %q, image: %q)", key, cacheAddr),
		trace.WithAttributes(
			attribute.String("cache.key", key),
			attribute.String("cache.image", cacheAddr),
		),
	)
	defer telemetry.EndWithCause(span, nil)

	_, err := dag.Container().WithDirectory("", dir).Publish(ctx, cacheAddr)

	return err
}

func cacheKeyAddr(registry string, repo string, key string) string {
	return fmt.Sprintf("%s/%s:%s", registry, repo, key)
}
