# Remote Cache

Remotely cache Dagger volumes and directories.

> [!NOTE]
> This doesn't work as a module right now because modules cannot export
> functions which return `dagger.WithContainerFunc`. Until then, this module
> remains purely as an example project.

## Example

Begin by generating a cache key for the example.

```sh
export CACHE_KEY=$(uuidgen)
```

Let's check the cache to ensure it doesn't already exist. We should see `CACHE MISS` be printed.

```sh
dagger call -m github.com/cbochs/dagger-modules/remote-cache \
    --registry ttl.sh \
    --repo $CACHE_KEY \
    --cache-key 2h \
    check-cache
```

Next, let's prime the cache with a file called `foo` and the contents `Hello, world`.

```sh
dagger call -m github.com/cbochs/dagger-modules/remote-cache \
    --registry ttl.sh \
    --repo $CACHE_KEY \
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
    --repo $CACHE_KEY \
    --cache-key 2h \
    check-cache
# Prints: Hello, world!
```
