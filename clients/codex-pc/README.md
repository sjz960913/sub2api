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

The Rust shell now includes a bounded stdio adapter for the stable Codex
app-server API. It initializes with `experimentalApi: false`, normalizes thread
metadata before exposing it to the desktop UI, starts turns without changing
the configured sandbox, and rejects unexpected approval/user-input requests.
There is no PC approval or confirmation flow.

The OS-secret-store facade still does not persist secrets until a real platform
keyring adapter is wired and tested.
