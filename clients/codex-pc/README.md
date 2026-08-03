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

CI builds a Linux Debian installer and publishes `sub2api-codex-pc-deb` as a seven-day workflow
artifact. Windows and macOS bundles use the generated `.ico` and `.icns` assets in
`src-tauri/icons`.

The Rust shell now includes a bounded stdio adapter for the stable Codex
app-server API. It initializes with `experimentalApi: false`, normalizes thread
metadata before exposing it to the desktop UI, starts turns without changing
the configured sandbox, and rejects unexpected approval/user-input requests.
There is no PC approval or confirmation flow.

Panel email/password login, TOTP, refresh rotation, device registration, WebSocket relay, and the
Codex app-server adapter are connected. Refresh tokens are stored through the native platform
credential backend: Windows Credential Manager, macOS Keychain, or Linux Secret Service. Access
tokens remain process-local, and passwords are never persisted.
