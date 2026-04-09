# Releasing Polvo

## Required GitHub Secrets

Configure in **Settings → Secrets and variables → Actions**:

| Secret | Description |
|--------|-------------|
| `TAURI_SIGNING_PRIVATE_KEY` | Ed25519 private key for signing update bundles |
| `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | Password for the private key |
| `APPLE_CERTIFICATE` | Base64-encoded Apple Developer ID `.p12` certificate |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the `.p12` file |
| `APPLE_SIGNING_IDENTITY` | e.g. `Developer ID Application: Your Name (XXXXXXXXXX)` |
| `APPLE_ID` | Apple ID email for notarization |
| `APPLE_PASSWORD` | App-specific password (appleid.apple.com) |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `HOMEBREW_TAP_TOKEN` | GitHub PAT with write access to `co2-lab/homebrew-tap` |
| `WINGET_TOKEN` | GitHub PAT for winget-releaser submissions |

`GITHUB_TOKEN` is provided automatically by GitHub Actions.

---

## One-time Setup

### 1. Tauri signing key

```sh
cargo tauri signer generate -w ~/.tauri/polvo.key
```

- Copy `~/.tauri/polvo.key` → `TAURI_SIGNING_PRIVATE_KEY` secret
- Copy `~/.tauri/polvo.key.pub` → `desktop/tauri.conf.json` under `plugins.updater.pubkey`

### 2. Homebrew tap

Create a public repo `co2-lab/homebrew-tap` with the structure:
```
homebrew-tap/
  Casks/
    polvo.rb   # auto-updated by release workflow
```

Generate a GitHub PAT with `repo` scope on that repository → `HOMEBREW_TAP_TOKEN` secret.

Users will install via:
```sh
brew install --cask co2-lab/tap/polvo
```

### 3. winget

The first submission to winget requires a manual PR to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs). After that, `winget-releaser` handles subsequent updates automatically via `WINGET_TOKEN`.

---

## Commit Convention

Commits must follow [Conventional Commits](https://www.conventionalcommits.org/) so `git-cliff` can generate release notes automatically:

| Prefix | Effect |
|--------|--------|
| `feat: ...` | Appears under **Features** |
| `fix: ...` | Appears under **Bug Fixes** |
| `perf: ...` | Appears under **Performance** |
| `breaking: ...` | Appears under **Breaking Changes** |
| `chore:`, `docs:`, `ci:`, `test:` | Skipped (not in release notes) |

---

## Triggering a Release

```sh
make release TAG=v0.1.0
```

This tags the commit and pushes to `origin`, which triggers the `release.yml` workflow.

### What happens automatically

1. **`release-notes`** — generates release notes from commits since last tag using `git-cliff`
2. **`build-tauri`** — builds on macOS (universal DMG), Linux (AppImage + .deb), Windows (MSI) with code signing and notarization; creates a **draft** GitHub release with the generated notes
3. **`generate-update-manifest`** — generates `latest.json` for the in-app auto-updater and uploads it to the release
4. **`update-changelog`** — updates `CHANGELOG.md` and commits it to `main`
5. **`publish-homebrew`** — updates `co2-lab/homebrew-tap` with the new version and SHA256
6. **`publish-winget`** — submits the new version to the winget repository

The release is created as a **draft**. Review it on GitHub and publish manually when ready.

---

## Distribution Channels

| Channel | Platform | Command |
|---------|----------|---------|
| GitHub Releases | All | Direct download |
| Homebrew | macOS | `brew install --cask co2-lab/tap/polvo` |
| winget | Windows | `winget install co2-lab.Polvo` |
| AppImage | Linux | Direct download |
| .deb | Linux (Debian/Ubuntu) | Direct download |
| Auto-updater | All | Built-in (Tauri updater) |
