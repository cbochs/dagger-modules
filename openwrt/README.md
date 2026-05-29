# OpenWRT

```sh
dagger call \
    with-non-privileged-user --name cbochs --pubkey ~/.ssh/id_ed25519.pub \
    build --packages "kmod-usb-net-asix-ax88179,luci,owut" \
    export --path images
```
