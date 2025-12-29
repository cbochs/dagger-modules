package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"dagger/remote-cache/internal/dagger"
	"dagger/remote-cache/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type RemoteCache struct {
	Backend *Backend       // +private
	Mounts  []*VolumeMount // +private
}

type VolumeMount struct {
	Path      string
	Key       string
	Container *dagger.Container                      // +private
	Platform  dagger.Platform                        // +private
	Volume    *dagger.CacheVolume                    // +private
	Opts      []dagger.ContainerWithMountedCacheOpts // +private
}

func New(registry string, repo string) *RemoteCache {
	backend := NewRegistry(registry, repo)
	return &RemoteCache{
		Backend: backend,
		Mounts:  nil,
	}
}

type PendingMount struct {
	Cache *RemoteCache // +private
	Mnt   *VolumeMount // +private
}

// +cache="session"
func (m *RemoteCache) CacheVolume(
	path string,
	key string,
	owner string, // +optional
) PendingMount {
	var opts []dagger.ContainerWithMountedCacheOpts
	if owner != "" {
		opts = append(opts, dagger.ContainerWithMountedCacheOpts{Owner: owner})
	}

	mnt := &VolumeMount{
		Path:   path,
		Key:    key,
		Volume: dag.CacheVolume(key),
		Opts:   opts,
		// Container: ctr,
	}

	return PendingMount{m, mnt}
}

// +cache="session"
func (pm PendingMount) Mount(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	platform, err := ctr.Platform(ctx)
	if err != nil {
		return nil, err
	}

	pm.Mnt.Container = ctr
	pm.Mnt.Platform = platform

	f := func(vm *VolumeMount, t *VolumeMount) int { return strings.Compare(vm.Key, t.Key) }
	if idx, exists := slices.BinarySearchFunc(pm.Cache.Mounts, pm.Mnt, f); !exists {
		_, span := Tracer().Start(ctx, fmt.Sprintf("inserting at index [%d]", idx))
		pm.Cache.Mounts = slices.Insert(pm.Cache.Mounts, idx, pm.Mnt)
		telemetry.EndWithCause(span, nil)
	}

	// arch := strings.Split(string(pm.Mnt.Platform), "/")[1]
	// key := fmt.Sprintf("%s-%s", pm.Mnt.Key, arch)
	key := fmt.Sprintf("%s", pm.Mnt.Key)
	dir, err := pm.Cache.Backend.Import(ctx, key)
	if err != nil {
		return nil, err
	}

	// Copy contents of dir to cache
	copyCtx, copySpan := Tracer().Start(ctx, "copyCache")
	_, err = pm.Mnt.Container.
		WithMountedCache(pm.Mnt.Path, pm.Mnt.Volume, pm.Mnt.Opts...).
		WithMountedDirectory("/cache", dir).
		WithExec([]string{"find", pm.Mnt.Path, "-mindepth", "1", "-delete"}).
		WithExec([]string{
			"cp",
			"-r", // copy directory recursively
			"-T", // avoid creating subdirectory
			"-p", // preserve mode, ownership, and timestamps
			"/cache",
			pm.Mnt.Path,
		}).
		Sync(copyCtx)
	if err != nil {
		telemetry.EndWithCause(copySpan, &err)
		return nil, err
	}
	telemetry.EndWithCause(copySpan, nil)

	_, mountSpan := Tracer().Start(
		ctx,
		fmt.Sprintf("withMountedCache(path: %q, key: %q)", pm.Mnt.Path, pm.Mnt.Key),
		trace.WithAttributes(
			attribute.String("cache.path", pm.Mnt.Path),
			attribute.String("cache.key", pm.Mnt.Key),
		),
	)
	ctr = ctr.WithMountedCache(pm.Mnt.Path, pm.Mnt.Volume, pm.Mnt.Opts...)
	telemetry.EndWithCause(mountSpan, nil)

	return ctr, nil
}

// +cache="session"
func (m *RemoteCache) Export(ctx context.Context) error {
	if len(m.Mounts) == 0 {
		return nil
	}

	ctx, span := Tracer().Start(ctx, "RemoteCache.export")
	defer telemetry.EndWithCause(span, nil)

	for _, mnt := range m.Mounts {
		arch := strings.Split(string(mnt.Platform), "/")[1]

		_, prepSpan := Tracer().Start(ctx, fmt.Sprintf("prepare cache directory: %s", mnt.Path))
		// Copy contents of cache to dir
		dir := mnt.Container.
			WithMountedCache(mnt.Path, mnt.Volume, mnt.Opts...).
			WithExec([]string{"cp", "-rTp", mnt.Path, "/tmp/cache"}).
			Directory("/tmp/cache")
		telemetry.EndWithCause(prepSpan, nil)

		key := fmt.Sprintf("%s-%s-%s", mnt.Path, mnt.Key, arch)
		span.SetAttributes(attribute.String("cache.key", key))
		if err := m.Backend.Export(ctx, key, dir); err != nil {
			return err
		}
	}

	return nil
}

// +cache="session"
func (m *RemoteCache) InlineExport(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	var err error
	ctr, err = ctr.Sync(ctx)
	if err != nil {
		return nil, err
	}
	err = m.Export(ctx)
	if err != nil {
		return nil, err
	}
	return ctr, nil
}
