# Sub2API Mobile

Android-first Flutter client for Panel chat, GPT Image, API-key selection, account actions, and
Codex PC collaboration. The application uses the approved four-tab visual system: 聊天、协同、
秘钥、我的。

Chat history is stored only in the app-private SQLite database on the current device. Records are
partitioned by Panel origin and user ID, and contain conversation metadata, model names, redacted
message text and generated-image payloads only. API-key/JWT-shaped text is redacted before every
write, and query/fragment credentials are removed from remote image URLs. The schema deliberately
has no API-key, secret-key, selected-key ID or selected-key name fields. Reopening a conversation
always uses the currently selected in-memory API key for the next gateway request. Users can open
or delete records from the 聊天 history list.

## Local verification

```bash
flutter pub get
flutter gen-l10n
flutter analyze
flutter test
flutter build apk --debug
```

CI runs the same checks and publishes `sub2api-mobile-debug-apk` as a seven-day workflow artifact.

For a reproducible Linux-hosted Android build, build `Dockerfile.android-builder`, mount an official
Flutter SDK at `/opt/flutter-current`, and prepend that SDK to `PATH`. The image provides Java 17,
Android SDK/Build Tools 35–36, NDK 28.2 and CMake 3.22.1; keep the Flutter SDK and Pub/Gradle caches
outside disposable containers.

## Release signing

Release builds never fall back to the Android debug key. Supply all four variables from a protected
CI environment or local secret manager before building an AAB:

```bash
export SUB2API_ANDROID_KEYSTORE=/absolute/path/to/release.jks
export SUB2API_ANDROID_STORE_PASSWORD='...'
export SUB2API_ANDROID_KEY_ALIAS='...'
export SUB2API_ANDROID_KEY_PASSWORD='...'
flutter build appbundle --release
```

Do not commit the keystore or any signing value.

Android `minSdk` follows the current stable Flutter toolchain default because the selected
secure-storage implementation uses the modern Android cryptography path. Never place Panel refresh
tokens or full API keys in SQLite, logs or crash reports.
