package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dagger/remote-cache/internal/dagger"
	"dagger/remote-cache/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
)

const mountLabelPrefix = "dagger.remote-cache.mount."

type RemoteCache struct {
	Backend Backend // +private
}

type VolumeMount struct {
	Cache  *RemoteCache                           // +private
	Meta   MountMetadata                          // +private
	Volume *dagger.CacheVolume                    // +private
	Opts   []dagger.ContainerWithMountedCacheOpts // +private
}

type MountMetadata struct {
	Path          string
	Key           string
	PlatformAware bool
}

func New(backend Backend) *RemoteCache {
	return &RemoteCache{
		Backend: backend,
	}
}

// +cache="session"
func (m *RemoteCache) CacheVolume(
	// Mount path.
	path string,
	// Cache volume key.
	key string,
	// A user:group to set for the mounted cache directory.
	// +optional
	owner string,
	// Replace "${VAR}" or "$VAR" in the value of path according to the current environment variables defined in the container (e.g. "/$VAR/foo").
	// +optional
	expand bool,
	// +optional
	platformAware bool,
) VolumeMount {
	var opts []dagger.ContainerWithMountedCacheOpts
	if owner != "" {
		opts = append(opts, dagger.ContainerWithMountedCacheOpts{Owner: owner})
	}
	if expand {
		opts = append(opts, dagger.ContainerWithMountedCacheOpts{Expand: expand})
	}

	return VolumeMount{
		Cache: m,
		Meta: MountMetadata{
			Path:          path,
			Key:           key,
			PlatformAware: platformAware,
		},
		Volume: dag.CacheVolume(key),
		Opts:   opts,
	}
}

// +cache="session"
func (mnt VolumeMount) Mount(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	ctr = ctr.WithMountedCache(mnt.Meta.Path, mnt.Volume, mnt.Opts...)

	// Copy the contents of the imported cache directory into the cache volume.
	key, err := labelKey(ctx, ctr, mnt.Meta)
	if err != nil {
		return nil, err
	}

	tmp := "/tmp/cache-import-" + key
	dir := mnt.Cache.Backend.Import(ctx, key)

	_, err = ctr.
		WithMountedDirectory(tmp, dir).
		WithExec([]string{"find", mnt.Meta.Path, "-mindepth", "1", "-delete"}).
		WithExec([]string{
			"cp",
			"-r", // copy directory recursively
			"-T", // avoid creating subdirectory
			"-p", // preserve mode, ownership, and timestamps
			tmp,
			mnt.Meta.Path,
		}).
		WithoutMount(tmp).
		Sync(ctx)
	if err != nil {
		return ctr, nil
	}

	metadata := MountMetadata{
		Path:          mnt.Meta.Path,
		Key:           mnt.Meta.Key,
		PlatformAware: mnt.Meta.PlatformAware,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return ctr, nil
	}

	ctr = ctr.WithLabel(mountLabelPrefix+key, string(metadataJSON))

	return ctr, nil
}

// +cache="session"
func (m *RemoteCache) Export(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	ctr, err := ctr.Sync(ctx)
	if err != nil {
		return nil, err
	}

	labels, err := ctr.Labels(ctx)
	if err != nil {
		return nil, err
	}

	for _, label := range labels {
		labelName, err := label.Name(ctx)
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(labelName, mountLabelPrefix) {
			continue
		}

		_, exportSpan := Tracer().Start(ctx, fmt.Sprintf("exportCache(name: %q)", labelName))

		labelValue, err := label.Value(ctx)
		if err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			return nil, err
		}

		var meta MountMetadata
		if err := json.Unmarshal([]byte(labelValue), &meta); err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			return nil, err
		}
		exportSpan.SetAttributes(
			attribute.String("cache.path", meta.Path),
			attribute.String("cache.key", meta.Key),
		)

		// Extract cache volume and copy to directory
		key, err := labelKey(ctx, ctr, meta)
		if err != nil {
			return nil, err
		}

		tmp := "/tmp/cache-export-" + key
		dir := ctr.
			WithExec([]string{"cp", "-rTp", meta.Path, tmp}).
			Directory(tmp)

		if err := m.Backend.Export(ctx, key, dir); err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			continue
		}

		// Remove label
		ctr = ctr.WithoutLabel(labelName)

		telemetry.EndWithCause(exportSpan, nil)
	}

	return ctr, nil
}

func labelKey(ctx context.Context, ctr *dagger.Container, meta MountMetadata) (string, error) {
	moduleName, err := dag.CurrentModule().Name(ctx)
	if err != nil {
		return "", err
	}

	var platformSuffix string
	if meta.PlatformAware {
		platform, err := ctr.Platform(ctx)
		if err != nil {
			return "", err
		}
		platformSuffix = "-" + strings.ReplaceAll(string(platform), "/", "-")
	}

	return fmt.Sprintf("%s-%s%s", moduleName, meta.Key, platformSuffix), nil
}
