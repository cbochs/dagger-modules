#!/usr/bin/env sh

docker stop registry
sleep 2
docker run --rm --detach \
    --name registry \
    --volume var-lib-docker:/var/lib/docker \
    -p 5000:5000 \
    ghcr.io/project-zot/zot-linux-arm64:latest

docker stop dagger-dev
sleep 2
docker run --rm --privileged --detach \
    --name dagger-dev \
    --link registry:registry \
    --volume var-lib-dagger:/var/lib/dagger \
    --volume ./engine.toml:/etc/dagger/engine.toml \
    registry.dagger.io/engine:v0.19.8

export _EXPERIMENTAL_DAGGER_RUNNER_HOST="docker-container://dagger-dev"
