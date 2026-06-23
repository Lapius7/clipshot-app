# clipshot-app

[日本語版 README はこちら / Japanese README](README.ja.md)

A Windows tray client written in Go. Press a hotkey, and whatever image is currently on your clipboard gets uploaded to your own [clipshot-server](https://github.com/Lapius7/clipshot-server) instance, with the resulting URL written straight back onto your clipboard — the same "press a key, paste a link" workflow as ShareX or Gyazo, pointed at a server you run yourself.

- Lives in the system tray, no visible window most of the time.
- Two ways to upload: the global hotkey (clipboard image) or **Upload Image...** in the tray menu (native file picker, any image on disk).
- The hotkey combo is configurable from the Settings dialog — no manual config file editing required.
- The API token never touches disk in plaintext — it's stored in the Windows Credential Manager (DPAPI-backed).
- Refuses to talk to anything that isn't `https://`.
- No CGO anywhere in the dependency tree, so it cross-compiles cleanly from Linux/WSL.

## Table of contents

- [Why this exists](#why-this-exists)
- [How it works](#how-it-works)
- [Installing / building](#installing--building)
- [First-time setup](#first-time-setup)
- [Usage](#usage)
- [Where settings and secrets are stored](#where-settings-and-secrets-are-stored)
- [Security model](#security-model)
- [Troubleshooting](#troubleshooting)
- [Project layout](#project-layout)
- [Downloads & verifying releases](#downloads--verifying-releases)
- [Roadmap / known limitations](#roadmap--known-limitations)
- [Contributing](#contributing)
- [License](#license)

## Why this exists

This is the client half of the ClipShot project. The server half ([clipshot-server](https://github.com/Lapius7/clipshot-server)) is the self-hosted upload backend; this app is what actually sits on your Windows machine, watches for a hotkey, and does the upload. See that repo's README for the full picture of why this project exists instead of just using a hosted screenshot tool.

## How it works

There are two independent upload paths that converge on the same upload+clipboard-write logic:

**Hotkey path** (clipboard image):
```
 1. You copy/screenshot an image to the clipboard (CF_DIB format)
 2. You press the configured hotkey (default: Ctrl+Shift+U)
 3. clipshot-app reads the clipboard image, re-encodes it as PNG
 4. It loads your API token from Windows Credential Manager
 5. It POSTs the PNG to <instance-url>/api/upload over HTTPS
 6. The server returns {"url": "https://.../i/<id>.png"}
 7. clipshot-app overwrites the clipboard with that URL (as plain text)
 8. You paste the link wherever you needed it
```

**Tray menu path** (any file on disk):
```
 1. Right-click the tray icon → "Upload Image..."
 2. A native file picker opens, filtered to png/jpg/jpeg/gif/webp
 3. clipshot-app reads the chosen file and detects its content type
 4. Steps 4-8 above are identical (token load, upload, clipboard write)
```

Every step after triggering an upload happens in under a second on a typical broadband connection. If anything fails (no image on the clipboard, network error, server rejects the upload), the tray icon's tooltip briefly shows what went wrong instead of silently doing nothing.

## Installing / building

Pre-built `clipshot.exe` binaries are available from the [Releases page](https://github.com/Lapius7/clipshot-app/releases). There's no installer yet (see [Roadmap](#roadmap--known-limitations)) — you can also build from source.

### Requirements

- Go 1.23+
- A Windows machine to *run* the result on. You do **not** need Windows to *build* it — this project has no CGO dependencies, so cross-compiling from Linux/macOS/WSL works out of the box.

### Build

```bash
git clone https://github.com/Lapius7/clipshot-app.git
cd clipshot-app
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-H windowsgui -s -w" \
  -o clipshot.exe ./cmd/clipshot/...
```

- `-H windowsgui` suppresses the console window so it behaves like a proper tray app instead of popping up a terminal.
- `-s -w` strips debug symbols, shrinking the binary (cosmetic, optional).
- `CGO_ENABLED=0` is what makes this buildable from non-Windows hosts at all — every dependency (`getlantern/systray` on Windows, `golang.org/x/sys/windows`) is pure Go on this platform.

Copy `clipshot.exe` anywhere on the target Windows machine and run it.

> **Why no installer / code signing yet?** Currently only a bare `clipshot.exe` binary is distributed. An installer (Inno Setup) and code-signing certificate are planned — this will prevent Windows SmartScreen warnings. See [Roadmap](#roadmap--known-limitations).

## First-time setup

1. Run `clipshot.exe`. A tray icon appears — there's no main window.
2. Right-click the tray icon → **Settings**.
3. Enter the following three values:
   - **Instance URL** — your clipshot-server's public address, e.g. `https://img.example.com`. Must start with `https://`; anything else is rejected.
   - **API Token** — a token issued by your server's admin (`clipshot-server token create`, see [clipshot-server's README](https://github.com/Lapius7/clipshot-server#token-management)). Leave blank to keep whatever token is already stored for that instance URL.
   - **Hotkey** — the global key combo that triggers a clipboard upload, e.g. `Ctrl+Shift+U` (the default). Must be one or more of `Ctrl`/`Alt`/`Shift`/`Win` followed by a single character; invalid combos are rejected before saving.
4. On save, the URL and hotkey are written to a local config file and the token is written to the Windows Credential Manager. The hotkey is (re-)registered automatically — no restart needed.

## Usage

### Via hotkey (clipboard image)

1. Copy an image to the clipboard (a screenshot, an image copied from a browser, etc. — anything that places `CF_DIB` data on the clipboard).
2. Press your configured hotkey (default `Ctrl+Shift+U`).
3. The tray tooltip briefly shows the result (`ClipShot: URL copied to clipboard` on success, or an error message on failure).
4. Paste (`Ctrl+V`) wherever you need the link — it's already plain text on your clipboard.

### Via tray menu (any image file)

1. Right-click the tray icon → **Upload Image...**.
2. Pick any `.png`/`.jpg`/`.jpeg`/`.gif`/`.webp` file in the dialog.
3. Same result as above — the tray tooltip confirms success/failure and the URL lands on your clipboard.

### Changing the hotkey

Right-click the tray icon → **Settings**, and enter a new combo in the third prompt. The new hotkey takes effect immediately (the old one is unregistered first), no restart required.

Right-click the tray icon → **Quit** to exit cleanly (this unregisters the hotkey).

## Where settings and secrets are stored

| What | Where | Format |
|---|---|---|
| Instance URL, hotkey combo | `%APPDATA%\ClipShot\config.json` | Plaintext JSON (no secrets in this file) |
| API token | Windows Credential Manager, target name `ClipShot:<instance-url>` | DPAPI-encrypted, scoped to the current Windows user |

The token is deliberately kept out of `config.json` entirely — even if someone reads that file (or it ends up in a backup, a support ticket, a screenshot), there's no token in it. Each instance URL gets its own Credential Manager entry, so configuring multiple servers doesn't overwrite a previous token.

## Security model

- **Transport**: the uploader (`internal/uploader`) hard-rejects any instance URL that doesn't start with `https://` — there's no configuration flag to disable this. If your server isn't behind TLS yet, fix that first (see clipshot-server's README).
- **Secret storage**: the API token is saved via the Win32 Credential Manager APIs (`CredWriteW`/`CredReadW`), which encrypt the blob with DPAPI under the current user's profile. It cannot be read by other Windows user accounts on the same machine, and isn't stored in any file this app writes itself.
- **Native UI, no shell-outs**: the settings dialog is built directly on raw Win32 window/control APIs (`CreateWindowExW` etc.) rather than shelling out to PowerShell or similar to render a prompt — there's no string-interpolated command execution anywhere in the input path, which avoids an entire class of injection bugs that affect some "quick and easy" Windows dialog libraries.
- **No CGO**: every dependency in the Windows build is pure Go (`golang.org/x/sys/windows` syscalls, `getlantern/systray`'s pure-Go Windows backend). This keeps the supply chain and build process simple and auditable.
- **What this does *not* protect against**: if your Windows user account itself is compromised, the attacker can decrypt the stored token (DPAPI ties decryption to the logged-in user, not to this specific app). Treat your Windows login like you'd treat any other credential boundary.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Hotkey does nothing | Another application already registered the same global hotkey combo. Try a different one by editing `hotkey` in `config.json` (e.g. `"Ctrl+Alt+U"`), then restart clipshot-app. |
| Tooltip says "clipboard has no image" | The clipboard doesn't currently hold `CF_DIB` data — copying an image from most apps (browsers, Paint, screenshot tools) works; copying a *file* (e.g. from Explorer) generally does not. |
| Tooltip says "no API token configured" | Settings haven't been saved yet, or were saved for a different instance URL than the one currently in `config.json`. Re-open Settings and re-enter the token. |
| Settings rejects the hotkey | The combo must be one or more of `Ctrl`/`Alt`/`Shift`/`Win` joined by `+`, ending in exactly one character, e.g. `Ctrl+Shift+U`. Multi-character keys (`Ctrl+Enter`, function keys, etc.) aren't supported yet. |
| Upload fails with a 401-ish message | The token is wrong, was revoked, or doesn't match the instance URL you're pointed at. Re-issue a token server-side and re-enter it in Settings. |
| Upload fails with a size-related message | The image exceeds the server's `MAX_UPLOAD_MB`. This is a server-side limit; ask the server operator to raise it, or shrink the image. |

## Project layout

```
cmd/clipshot/main.go       Entry point: loads config, registers hotkey, starts the tray
cmd/clipshot/icon.go       Embeds the tray icon (icon.ico) into the binary
internal/config/           Settings load/save (%APPDATA%\ClipShot\config.json), URL validation
internal/credstore/        Windows Credential Manager wrapper (CredWriteW/CredReadW/CredDeleteW)
internal/hotkey/           Global hotkey registration via RegisterHotKey, combo validation
internal/clipboard/        Reads CF_DIB from the clipboard and re-encodes as PNG; writes URL text back
internal/uploader/         HTTPS-only multipart upload client
internal/ui/               Tray menu (getlantern/systray), native Win32 settings dialog, and native file picker (GetOpenFileNameW)
internal/notify/           User feedback via a temporary tray tooltip change
```

Non-Windows builds compile against stub implementations in `_other.go` files (returning "unsupported" errors) purely so the code is inspectable/testable from any OS — the app is only ever meant to run on Windows.

## Downloads & verifying releases

Prebuilt binaries are published on the [Releases page](https://github.com/Lapius7/clipshot-app/releases). **Do not download `clipshot.exe` from anywhere else** — there is no official mirror, no third-party download site, and no auto-updater. If you find this binary hosted elsewhere, treat it as untrusted.

### This binary is not code-signed

`clipshot.exe` is **not signed with a code-signing certificate** (see [Roadmap](#roadmap--known-limitations)). This means:

- Windows SmartScreen will very likely show an "unrecognized app" warning on first run. This is expected for an unsigned binary from a small open-source project, not necessarily a sign of tampering — but it also means Windows itself is not vouching for the publisher's identity the way it would for a signed binary.
- You are relying on the checksum below (and, ideally, building from source yourself) rather than a signature chain to know the binary matches what this repository produced.

If your threat model requires a verified publisher identity, build from source instead (see [Installing / building](#installing--building)) until signed releases are available.

### Verifying a downloaded binary

Every release includes a `SHA256SUMS.txt` alongside `clipshot.exe`. After downloading, verify the hash matches before running it:

**PowerShell:**
```powershell
Get-FileHash .\clipshot.exe -Algorithm SHA256
```

**Linux/macOS/WSL:**
```bash
sha256sum clipshot.exe
# or, to check directly against the published file:
sha256sum -c SHA256SUMS.txt
```

Compare the result against the matching line in that release's `SHA256SUMS.txt` on the [Releases page](https://github.com/Lapius7/clipshot-app/releases). If it doesn't match exactly, do not run the file — re-download it, and if the mismatch persists, open an issue.

### Reproducing a release build yourself

Since the build is just `go build` with no platform-specific toolchain quirks (no CGO), you can reproduce a given tag's binary and compare hashes yourself:

```bash
git clone https://github.com/Lapius7/clipshot-app.git
cd clipshot-app
git checkout v0.1.0   # match the release tag you downloaded
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H windowsgui -s -w" -o clipshot.exe ./cmd/clipshot/...
sha256sum clipshot.exe
```

Go's toolchain does not currently guarantee bit-for-bit reproducible builds across all environments/Go versions, so a mismatch here isn't automatically a red flag — but a match is strong independent confirmation that the published binary corresponds to this source tree.

## Roadmap / known limitations

This is an early-stage skeleton, not a finished product. Known gaps, roughly in priority order:

- [ ] A real installer (Inno Setup) instead of a bare `.exe`
- [ ] Code signing, so Windows SmartScreen doesn't warn on first run
- [ ] Auto-start on login (registry Run key or Task Scheduler registration)
- [ ] Support for multi-character hotkeys (function keys, Enter, etc.) — currently single-character only
- [ ] GitHub Actions CI for build verification and release artifacts
- [ ] Retry/backoff on transient network failures during upload
- [ ] Automated tests (current verification is manual: cross-compile check only — see commit history)

## Contributing

Issues and pull requests are welcome. This project intentionally stays small in scope — before proposing a large feature, consider opening an issue first.

## License

MIT (see `LICENSE`).
