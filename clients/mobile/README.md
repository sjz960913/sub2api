# Sub2API Mobile

Android-first Flutter client for Panel chat, GPT Image, API-key selection, account actions, and
Codex PC collaboration. The application uses the approved four-tab visual system: 聊天、协同、
秘钥、我的。

## Local verification

```bash
flutter pub get
flutter gen-l10n
flutter analyze
flutter test
flutter build apk --debug
```

CI runs the same checks and publishes `sub2api-mobile-debug-apk` as a seven-day workflow artifact.

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

Android `minSdk` is 23 because the selected secure-storage implementation uses
the modern Android cryptography path. Never place Panel refresh tokens or full
API keys in SQLite, logs or crash reports.
