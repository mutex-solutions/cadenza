# Releasing

Releases are built by GoReleaser when a version tag is pushed.

## Cut a release

1. Validate locally:

   ```sh
   make check
   (cd web && pnpm -r typecheck && pnpm -r test)
   make web
   goreleaser check
   goreleaser release --snapshot --clean --skip=publish
   ```

2. Tag and push:

   ```sh
   git tag -a v0.1.0 -m "cadenza v0.1.0"
   git push origin v0.1.0
   ```

`.github/workflows/release.yml` then:

- builds the web app and embeds it in the server,
- builds `cadenza` for linux, darwin and windows and `cadenza-server` for linux and darwin, amd64 and arm64,
- publishes `cadenza_<os>_<arch>.tar.gz` (zip on Windows), `cadenza-server_<os>_<arch>.tar.gz` and
  `checksums.txt` as a GitHub release,
- pushes the server image to `ghcr.io/mutex-solutions/cadenza`,
- regenerates `Formula/cadenza.rb` in `mutex-solutions/homebrew-tap`.

`install.sh`, `install.ps1` and `brew install mutex-solutions/tap/cadenza` read from that release.

## Version

The version comes from the tag through `-ldflags -X main.Version`. Check it after a release:

```sh
cadenza version
cadenza-server version
```

When a release changes the API in a way older CLIs cannot handle, raise `CADENZA_MIN_CLI_VERSION`'s default in
`internal/server/config.go`; older CLIs are then told to update.

## Android app

Each version tag also runs `.github/workflows/android.yml`, which builds `cadenza-android-arm64.apk` and attaches it
to the release, for installing on a phone with `adb install`. That APK is signed with the build's debug key: fine
for your own devices, not for the Play Store. To attach an APK to an existing release, run the workflow by hand
with the tag.

For the Play Store, the Android app is built with EAS from `web/apps/android`; see its README. Google Play requires a yearly target
SDK update: plan it into the yearly upgrade window with the other dependency updates.

## Homebrew tap

The workflow pushes the formula with a write deploy key on `mutex-solutions/homebrew-tap`, stored as the secret
`HOMEBREW_TAP_DEPLOY_KEY`. Without the secret the step is skipped and the release still succeeds.
