# sandstorm-utils testapp

This is a minimal Sandstorm integration harness for the utilities in this repo.

It is intended to be packaged as a Sandstorm app and used to verify that the
compiled tools work correctly inside a real grain. The app:

- exposes a tiny UI over HTTP
- invokes the utilities as subprocesses
- captures stdout, stderr, exit status, and parsed JSON where appropriate
- keeps recent scenario results in memory for inspection
- is used by the VM-backed `app-harness serve --pkg-def` integration test

The package build is owned by [`.sandstorm/build.sh`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/build.sh),
which compiles every utility plus the testapp binaries inside the Sandstorm VM
during `spktool dev` and `spktool pack`.

The package definition in [`.sandstorm/sandstorm-pkgdef.capnp`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/sandstorm-pkgdef.capnp)
currently contains a placeholder package ID. Replace it with the package ID for
the signing key stored in the `SANDSTORM_TESTAPP_KEYRING_B64` GitHub secret
before expecting the SPK workflow to succeed.

For local Sandstorm development, run `spktool` from the repo root, not from
`testapp/`, so the VM sees the full Go module at `/opt/app`.

Typical local flow:

```bash
spktool vm up
spktool vm ssh -- /bin/sh -lc 'cd /opt/app && bash .sandstorm/build.sh'
spktool vm ssh -- /bin/sh -lc '/opt/app/testapp/bin/app-harness serve --root /tmp/gh-serve --pkg-def /opt/app/.sandstorm/sandstorm-pkgdef.capnp:pkgdef --port 3010'
```

If you change [`.sandstorm/box.toml`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/box.toml),
[`.sandstorm/build.sh`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/build.sh),
[`.sandstorm/setup.sh`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/setup.sh), or
[`.sandstorm/launcher.sh`](/Users/mnutt/p/personal/sandstorm-utils/.sandstorm/launcher.sh),
rerender the managed files with:

```bash
spktool upgradevm
```

From another shell, you can hit the served app directly inside the VM:

```bash
spktool vm ssh -- /bin/sh -lc 'curl -i http://127.0.0.1:3010/'
```
