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
	Cache  *RemoteCache        // +private
	Meta   MountMetadata       // +private
	Volume *dagger.CacheVolume // +private
}

type MountMetadata struct {
	Path          string
	Key           string
	PlatformAware bool
	Owner         string
	Expand        bool
	CacheExists   bool
	ForceExport   bool
}

func New(backend Backend) *RemoteCache {
	return &RemoteCache{
		Backend: backend,
	}
}

func (m *RemoteCache) Mount(
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
	// +optional
	force bool,
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
			Owner:         owner,
			Expand:        expand,
			PlatformAware: platformAware,
			CacheExists:   false,
			ForceExport:   force,
		},
		Volume: dag.CacheVolume(key),
	}
}

func (mnt VolumeMount) AsCacheVolume(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	meta := mnt.Meta

	key, err := labelKey(ctx, ctr, meta)
	if err != nil {
		return nil, err
	}

	dir := mnt.Cache.Backend.Import(ctx, key)
	entries, err := dir.Entries(ctx)
	if len(entries) > 0 {
		meta.CacheExists = true
	}

	metadataJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	ctr = ctr.
		WithLabel(mountLabelPrefix+key, string(metadataJSON)).
		WithMountedCache(meta.Path, mnt.Volume, dagger.ContainerWithMountedCacheOpts{
			Owner:  meta.Owner,
			Expand: meta.Expand,
		})

	if meta.CacheExists {
		tmp := "/tmp/cache-import-" + key
		_, err = ctr.
			WithMountedDirectory(tmp, dir).
			WithExec([]string{"find", meta.Path, "-mindepth", "1", "-delete"}).
			WithExec([]string{
				"cp",
				"-r", // copy directory recursively
				"-T", // avoid creating subdirectory
				"-p", // preserve mode, ownership, and timestamps
				tmp,
				meta.Path,
			}).
			WithoutMount(tmp).
			Sync(ctx)
		if err != nil {
			return nil, err
		}
	}

	return ctr, nil
}

func (mnt VolumeMount) AsDirectory(ctx context.Context, ctr *dagger.Container) (*dagger.Container, error) {
	meta := mnt.Meta

	key, err := labelKey(ctx, ctr, meta)
	if err != nil {
		return nil, err
	}

	dir := mnt.Cache.Backend.Import(ctx, key)
	if entries, _ := dir.Entries(ctx); len(entries) > 0 {
		meta.CacheExists = true
	}

	metadataJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	ctr = ctr.
		WithLabel(mountLabelPrefix+key, string(metadataJSON)).
		WithDirectory(meta.Path, dir, dagger.ContainerWithDirectoryOpts{
			Owner:  meta.Owner,
			Expand: meta.Expand,
		})

	return ctr, nil
}

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
			continue // Don't err on export
		}

		if !strings.HasPrefix(labelName, mountLabelPrefix) {
			continue
		}

		_, exportSpan := Tracer().Start(ctx, fmt.Sprintf("exportCache(name: %q)", labelName))

		labelValue, err := label.Value(ctx)
		if err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			continue // Don't err on export
		}

		var meta MountMetadata
		if err := json.Unmarshal([]byte(labelValue), &meta); err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			continue // Don't err on export
		}
		exportSpan.SetAttributes(
			attribute.String("cache.path", meta.Path),
			attribute.String("cache.key", meta.Key),
		)

		key, err := labelKey(ctx, ctr, meta)
		if err != nil {
			continue // Don't err on export
		}

		// Skip export for existing cache entries
		if meta.CacheExists && !meta.ForceExport {
			telemetry.EndWithCause(exportSpan, nil)

			_, skipSpan := Tracer().Start(ctx, fmt.Sprintf("cache exists for %q, skipping", key))
			telemetry.EndWithCause(skipSpan, nil)

			continue
		}

		tmp := "/tmp/cache-export-" + key
		dir := ctr.
			WithExec([]string{"cp", "-rTp", meta.Path, tmp}).
			Directory(tmp)

		if err := m.Backend.Export(ctx, key, dir); err != nil {
			telemetry.EndWithCause(exportSpan, &err)
			continue // Don't err on export
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
