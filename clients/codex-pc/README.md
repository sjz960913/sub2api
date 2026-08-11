# Codex PC Companion

Tauri 2 + React companion for safely relaying selected Codex CLI sessions to
the authenticated Sub2API user.

The desktop navigation intentionally contains only **概览、会话、实时任务、设置**.
There is no approval queue. Unexpected Codex approval requests are rejected by
the non-interactive local safety policy in the adapter milestone.

```bash
npm install
npm run build
npm run tauri build
```

For a reproducible Ubuntu 22.04+ Debian package without installing Node/Rust on the host:

```bash
docker build -f Dockerfile.linux-builder -t sub2api-codex-pc-builder .
docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp/build-home \
  -v "$PWD:/workspace" -w /workspace \
  sub2api-codex-pc-builder \
  bash -lc 'npm ci && npm run tauri -- build --bundles deb'
```

The package is written to `src-tauri/target/release/bundle/deb/`. Windows NSIS installers must be
built on a native Windows runner using the repository workflow; a Linux cross-build is not treated
as a verified Windows deliverable.

CI builds Linux Debian and Windows NSIS installers, publishing `sub2api-codex-pc-deb` and
`sub2api-codex-pc-windows` as seven-day workflow artifacts. macOS bundles can use the generated
`.icns` asset in `src-tauri/icons` when a signing/notarization environment is available.

The Rust shell now includes a bounded stdio adapter for the stable Codex
app-server API. It initializes with `experimentalApi: false`, normalizes thread
metadata before exposing it to the desktop UI, starts turns without changing
the configured sandbox, and rejects unexpected approval/user-input requests.
There is no PC approval or confirmation flow.

Panel email/password login, TOTP, refresh rotation, device registration, WebSocket relay, and the
Codex app-server adapter are connected. Refresh tokens are stored through the native platform
credential backend: Windows Credential Manager, macOS Keychain, or Linux Secret Service. Access
tokens remain process-local, and passwords are never persisted.
