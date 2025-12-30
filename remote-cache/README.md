# Remote Cache

A Dagger module for remotely caching Dagger volumes and directories with pluggable storage backends.

## Overview

This module provides a flexible caching system that allows you to persist Dagger cache volumes to remote storage backends. It's designed as a drop-in replacement for `WithMountedCache` with support for multiple storage backends through the `Backend` interface.

## Architecture

The module uses a modular architecture:

- **RemoteCache**: Core caching module that manages cache volume imports/exports
- **Backend Interface**: Pluggable storage backend interface with `Import` and `Export` methods
- **Example Backends**: See the `remote-backend` module for an OCI registry implementation

## Usage

### Basic Example

```go
// Create a backend (e.g., OCI registry)
backend := dag.RemoteBackend().Registry("ttl.sh", "my-cache-repo")

// Initialize the cache with the backend
cache := dag.RemoteCache(backend.AsRemoteCacheBackend())

// Mount a cache volume
ctr := dag.Container().
    From("alpine").
    With(cache.CacheVolume("/example", "my-cache-key").Mount).
    WithExec([]string{"npm", "install"})

// Export the cache after building
ctr, err := cache.Export(ctx, ctr)
if err != nil {
    return nil, err
}
```

## Implementing a Custom Backend

To create your own storage backend, implement the `Backend` interface:

```go
type Backend interface {
    DaggerObject

    Import(ctx context.Context, key string) *dagger.Directory
    Export(ctx context.Context, key string, dir *dagger.Directory) error
}
```

See the [`remote-backend`](../remote-backend) module for an example OCI registry implementation.

## Example

The `example` subdirectory contains a working example. Here's how to test it:

### 1. Generate a unique cache repo

```sh
export CACHE_REPO=$(uuidgen)
```

### 2. Check the cache (should show CACHE MISS)

> [!NOTE]
> When using `ttl.sh`, the cache key becomes the image tag. For example, a cache
> key of `2h` sets the image expiry to 2 hours.

```sh
dagger call -m ./remote-cache/example \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    check-cache
```

### 3. Prime the cache with test data

```sh
dagger call -m ./remote-cache/example \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    prime-cache
```

### 4. Prune the local Dagger cache

```sh
dagger core engine local-cache prune
```

### 5. Verify cache retrieval from remote storage

```sh
dagger call -m ./remote-cache/example \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    check-cache
# Prints: Hello, world
```

At this point, the cache volume has been successfully retrieved from remote storage after being cleared locally.
