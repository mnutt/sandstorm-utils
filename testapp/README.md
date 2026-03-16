# sandstorm-utils testapp

This is a minimal Sandstorm integration harness for the utilities in this repo.

It is intended to be packaged as a Sandstorm app and used to verify that the
compiled tools work correctly inside a real grain. The app:

- exposes a tiny UI over HTTP
- invokes the utilities as subprocesses
- captures stdout, stderr, exit status, and parsed JSON where appropriate
- keeps recent scenario results in memory for inspection

The package build is owned by [`testapp/.sandstorm/build.sh`](/Users/mnutt/p/personal/sandstorm-utils/testapp/.sandstorm/build.sh),
which compiles every utility plus the testapp server inside the Lima VM during
SPK packaging.

The package definition in [`testapp/.sandstorm/sandstorm-pkgdef.capnp`](/Users/mnutt/p/personal/sandstorm-utils/testapp/.sandstorm/sandstorm-pkgdef.capnp)
currently contains a placeholder package ID. Replace it with the package ID for
the signing key stored in the `SANDSTORM_TESTAPP_KEYRING_B64` GitHub secret
before expecting the SPK workflow to succeed.

For local Sandstorm development, run `lima-spk` from the repo root, not from
`testapp/`. `lima-spk` mounts its work directory at `/opt/app`, so using the
repo root is what makes both `go.mod` and `testapp/` visible inside the VM.
