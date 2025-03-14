# Remote Cache

Remotely cache Dagger volumes and directories.

> [!WARNING]
> This doesn't work as a module right now because modules cannot export
> functions which return `dagger.WithContainerFunc`. Until then, this module
> remains purely as an example project.

### Expected Use-Case

This is expected to be an (almost) drop-in replacement for `WithMountedCache`.

For example, instead of

```go
dag.Container().WithMountedCache("/example", dag.CacheVolume("example"))
```

Use

```go
cache := NewCache().WithRemote("my-registry.com", "cache")

ctr := dag.Container().With(cache.MountedVolume("/example", "example"))

// Build the rest of your container...

ctr, err := cache.Sync(ctx, ctr)
if err != nil {
    return nil, err
}
```

## Example

Begin by generating a unique cache repo for the example.

```sh
export CACHE_REPO=$(uuidgen)
```

Let's check the cache to ensure it doesn't already exist. We should see `CACHE MISS` be printed.

> [!NOTE]
> Some of the input may be odd here. The cache key we choose will end up being
> the resulting image tag. In this case, since we're using `ttl.sh`, a cache
> key of `2h` (image expiry of 2 hours) is what we're inputting.

```sh
dagger call -m github.com/cbochs/dagger-modules/remote-cache \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    check-cache
```

Next, let's prime the cache with a file called `foo` and the contents `Hello, world`.

```sh
dagger call -m github.com/cbochs/dagger-modules/remote-cache \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    prime-cache
```

At this point the cache volume exists in Dagger and has also been published
remotely. Running `check-cache` again will confirm this. However, we're going
to skip that step and go straight to the pruning the volume from the local
Dagger cache.

```sh
dagger core engine local-cache prune
```

With the Dagger state removed we are now reliant on the cache volume existing
remotely. Let's see if we get can retrieve the contents we stored remotely.

```sh

dagger call -m github.com/cbochs/dagger-modules/remote-cache \
    --registry ttl.sh \
    --repo $CACHE_REPO \
    --cache-key 2h \
    check-cache
# Prints: Hello, world!
```
