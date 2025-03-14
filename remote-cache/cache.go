package main

import (
	"context"
	"dagger/remote-cache/internal/dagger"
	"fmt"
	"strings"

	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
)

type Cache struct {
	Registry     string                 // +private
	Repo         string                 // +private
	SkipIfExists bool                   // +private
	VolumeMounts map[string]VolumeMount // +private
	CacheKey     KeyFormatter           // +private
}

type VolumeMount struct {
	Key       string                                 // +private
	Path      string                                 // +private
	Volume    *dagger.CacheVolume                    // +private
	Container *dagger.Container                      // +private
	Opts      []dagger.ContainerWithMountedCacheOpts // +private
}

type KeyFormatter func(registry string, repo string, key string, arch string) string

func DefaultKeyFormatter(registry, repo, key, arch string) string {
	return fmt.Sprintf("%s/%s:%s-%s", registry, repo, key, arch)
}

func NewCache() *Cache {
	return &Cache{
		VolumeMounts: make(map[string]VolumeMount),
		CacheKey:     DefaultKeyFormatter,
	}
}

func (cache *Cache) WithRemote(registry string, repo string) *Cache {
	c := *cache
	c.Registry = registry
	c.Repo = repo
	return &c
}

func (cache *Cache) WithSkipIfExists() *Cache {
	c := *cache
	c.SkipIfExists = true
	return &c
}

func (cache *Cache) WithKeyFormatter(f KeyFormatter) *Cache {
	c := *cache
	c.CacheKey = f
	return &c
}

func (cache *Cache) RemoteEnabled() bool {
	return cache.Registry != ""
}

// Mount a cache volume to a container at the given path.
func (cache *Cache) MountedVolume(path string, key string, opts ...dagger.ContainerWithMountedCacheOpts) dagger.WithContainerFunc {
	return func(r *dagger.Container) *dagger.Container {
		mnt := VolumeMount{
			Key:       key,
			Path:      path,
			Volume:    dag.CacheVolume(key),
			Container: r,
			Opts:      opts,
		}
		cache.VolumeMounts[key] = mnt

		return r.WithMountedCache(mnt.Path, mnt.Volume, opts...)
	}
}

// Mount a cached directory to a container at the given path.
func (cache *Cache) MountedDirectory(path string, key string, opts ...dagger.ContainerWithMountedCacheOpts) dagger.WithContainerFunc {
	return func(r *dagger.Container) *dagger.Container {
		mnt := VolumeMount{
			Key:       key,
			Path:      path,
			Volume:    dag.CacheVolume(key),
			Container: r,
			Opts:      opts,
		}
		cache.VolumeMounts[key] = mnt

		cachePath := "/tmp/" + mnt.Key

		return r.
			WithMountedCache(cachePath, mnt.Volume, opts...).
			WithExec([]string{"cp", "-rTp", cachePath, mnt.Path})
	}
}

func (cache *Cache) Download(ctx context.Context) error {
	if !cache.RemoteEnabled() {
		return nil
	}

	for _, mnt := range cache.VolumeMounts {
		platform, err := mnt.Container.Platform(ctx)
		if err != nil {
			return err
		}
		arch := strings.Split(string(platform), "/")[1]

		imageAddr := cache.CacheKey(cache.Registry, cache.Repo, mnt.Key, arch)
		if !cache.imageExists(ctx, imageAddr) {
			continue
		}

		cacheDir := dag.Container().From(imageAddr).Directory("")
		_, err = mnt.Container.
			WithMountedCache(mnt.Path, mnt.Volume, mnt.Opts...).
			WithDirectory("/cache", cacheDir).
			WithExec([]string{"find", mnt.Path, "-mindepth", "1", "-delete"}).
			WithExec([]string{
				"cp",
				"-r", // copy directory recursively
				"-T", // avoid creating subdirectory
				"-p", // preserve mode, ownership, and timestamps
				"/cache",
				mnt.Path,
			}).
			Sync(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cache *Cache) Upload(ctx context.Context) error {
	if !cache.RemoteEnabled() {
		return nil
	}

	for _, mnt := range cache.VolumeMounts {
		platform, err := mnt.Container.Platform(ctx)
		if err != nil {
			return err
		}
		arch := strings.Split(string(platform), "/")[1]

		imageAddr := cache.CacheKey(cache.Registry, cache.Repo, mnt.Key, arch)
		if cache.imageExists(ctx, imageAddr) && cache.SkipIfExists {
			continue
		}

		cacheDir := mnt.Container.
			WithMountedCache(mnt.Path, mnt.Volume, mnt.Opts...).
			WithExec([]string{"cp", "-rTp", mnt.Path, "/tmp/cache"}).
			Directory("/tmp/cache")

		_, err = dag.
			Container(dagger.ContainerOpts{Platform: platform}).
			WithDirectory("", cacheDir).
			Publish(ctx, imageAddr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cache *Cache) Sync(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	var err error
	err = cache.Download(ctx)
	if err != nil {
		return nil, err
	}
	ctr, err = ctr.Sync(ctx)
	if err != nil {
		return nil, err
	}
	err = cache.Upload(ctx)
	if err != nil {
		return nil, err
	}
	return ctr, nil
}

func (cache *Cache) imageExists(ctx context.Context, address string) bool {
	imageName := "docker://" + address
	imageRef, err := alltransports.ParseImageName(imageName)
	if err != nil {
		return false
	}

	sys := &types.SystemContext{}
	imageSrc, err := imageRef.NewImageSource(ctx, sys)
	if err != nil {
		return false
	}
	defer imageSrc.Close()

	return true
}
