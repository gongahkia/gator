# Releasing

This project publishes release archives with GoReleaser. Darwin binaries are signed and notarized
before they are archived.

## Required Secrets

Configure these repository secrets before pushing a release tag:

| secret | value |
| --- | --- |
| `MACOS_CERTIFICATE` | base64 encoded Developer ID Application `.p12` |
| `MACOS_CERTIFICATE_PWD` | password for the `.p12` export |
| `MACOS_KEYCHAIN_PWD` | reserved for a native macOS keychain signing fallback |
| `MACOS_NOTARY_KEY` | base64 encoded App Store Connect API key `.p8` |
| `MACOS_NOTARY_KEY_ID` | App Store Connect key ID |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect issuer UUID |

## Apple Credential Setup

1. Enroll in the Apple Developer Program.
2. Create a `Developer ID Application` certificate.
3. Import the certificate into Keychain Access.
4. Export the certificate and private key as `Certificates.p12`; set a strong export password.
5. Create an App Store Connect API key with access to notarization.
6. Download the `ApiKey_<KEY_ID>.p8` file and record the key ID and issuer ID.

Encode the files for GitHub secrets:

```sh
base64 < Certificates.p12 | tr -d '\n' | pbcopy
base64 < ApiKey_KEYID.p8 | tr -d '\n' | pbcopy
```

Paste those values into `MACOS_CERTIFICATE` and `MACOS_NOTARY_KEY`.

## Release Flow

1. Confirm `make test` and `make lint` pass.
   On a WSL2 host, also run `make wsl-smoke`; this project does not publish or validate native
   Windows artifacts.
2. Confirm GoReleaser config is valid:

   ```sh
   goreleaser check
   ```

3. Push an annotated tag:

   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. The `release` workflow fails fast if signing/notarization secrets are missing.
5. GoReleaser signs and notarizes darwin binaries before packaging release archives.
6. The workflow publishes GitHub release assets.
7. The `macos-validate` job downloads the published darwin archives on macOS and runs
   `codesign -v --strict --verbose=2`, `codesign -dv --verbose=4`, and
   `spctl -a -t exec -vv` on each extracted `paw` binary.

## macOS Packaging Scope

The current pipeline uses GoReleaser's cross-platform macOS notarization path, which signs and
notarizes standalone Mach-O binaries before they are placed in release archives. This matches the
tarball install flow.

Do not add a stapling step to the current `tar.gz` archive path. If the project needs a stapled
distributable, add a separate `.pkg` or `.dmg` release path and validate it with `xcrun stapler`.
GoReleaser's built-in `.pkg` and `.dmg` builders are GoReleaser Pro features; without Pro, use a
separate macOS packaging job with `pkgbuild`/`productbuild` or `hdiutil`, followed by `notarytool`
and `stapler`.

## Verification

The release workflow verifies every darwin release archive after publication. After the first signed
release, also verify on a clean macOS 15+ machine:

```sh
curl -L -o paw.tar.gz https://github.com/gongahkia/paw-cli/releases/download/vX.Y.Z/paw_X.Y.Z_darwin_arm64.tar.gz
tar -xzf paw.tar.gz
codesign -dv --verbose=4 ./paw
codesign -v --strict --verbose=2 ./paw
spctl -a -t exec -vv ./paw
./paw version
```

Repeat for `darwin_amd64` on Intel macOS or under a matching validation host.

Current release archives are `tar.gz` files containing a signed/notarized Mach-O binary. There is no
`.app`, `.pkg`, or `.dmg` artifact to staple in this layout.

Linux release archives also serve WSL2. Validate them from a WSL2 checkout with `make wsl-smoke`;
there is no native Windows archive or CI guarantee.

## References

- Apple notarization: <https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution>
- GoReleaser notarization: <https://goreleaser.com/customization/sign/notarize/>
- GoReleaser DMG packaging: <https://goreleaser.com/customization/package/dmg/>
- GoReleaser PKG packaging: <https://goreleaser.com/customization/package/pkg/>
