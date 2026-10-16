# macOS release signing and notarization

The v0.3.0-electron.1 macOS download audit reproduced the reported failure. The Apple Silicon bundle retained an incomplete Electron ad-hoc signature after packaging: `codesign --verify --deep --strict` reports “code has no resources but signature indicates they must be present.” The Intel bundle has no usable signature. Gatekeeper rejects both and neither has a stapled notarization ticket. Launching an unquarantined app in CI did not catch these failures.

Normal public macOS releases must use a Developer ID Application certificate and Apple notarization. The release workflow now refuses to start without the required GitHub secrets and refuses publication if the downloaded ZIP fails signature, native-helper, notarization-ticket or Gatekeeper checks.

## Configure the release credentials

An Apple Developer Program account holder must create a **Developer ID Application** certificate, install it with its private key on their Mac, and export it from Keychain Access as a password-protected `.p12`. Use Developer ID Application, rather than Apple Development or a Mac App Store distribution certificate.

In the GitHub repository, open **Settings → Secrets and variables → Actions → New repository secret**, and configure:

| Name | Value |
| --- | --- |
| `CSC_LINK` | Base64-encoded contents of the exported `.p12` certificate |
| `CSC_KEY_PASSWORD` | Password used when exporting that `.p12` |
| `APPLE_ID` | Apple Developer account email used for notarization |
| `APPLE_APP_SPECIFIC_PASSWORD` | An Apple app-specific password for that account |
| `APPLE_TEAM_ID` | The account's Apple Developer team ID |

Keep these values in GitHub Secrets; do not put them in chat, commits, `.env` files in release artifacts, or logs. For maintainers using the GitHub CLI, certificate bytes can be streamed without printing them:

```sh
base64 < /path/to/DeveloperID.p12 | gh secret set CSC_LINK --repo JienWeng/6eek-go
```

Set the remaining values through the GitHub UI or the CLI's interactive secret prompts.

## Rebuild and verify

Run the **Electron candidate packages** workflow after all five secrets are configured. Version 0.3.1 signs the Electron app and its Go/FFmpeg/FFprobe executables, enables Hardened Runtime with Electron's JIT entitlement, submits the app to Apple, and staples the notarization ticket before packaging.

Both Mac architectures then extract the completed ZIP, apply downloaded-app quarantine metadata, verify the entire bundle and native helpers, validate the stapled ticket, require Gatekeeper acceptance, and launch the app in the smoke test. Publication of `v0.3.1-electron.1` occurs only after those checks and the Linux jobs pass. New signing happens before DMG/ZIP creation; editing a signed bundle afterward invalidates its seal.

The existing macOS downloads are not fixed by merely rerunning the old unsigned build. Ad-hoc signing can repair the bundle seal for local testing, but does not identify the developer or provide Apple notarization. Do not claim such a preview passes default Gatekeeper policy.

References: [Apple distribution packaging](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution), [Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution), [electron-builder v26 signing](https://www.electron.build/v26/docs/mac/).
