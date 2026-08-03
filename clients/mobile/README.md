# Sub2API Mobile

Android-first Flutter client. The current source contains the M0 application
shell and the approved four-tab visual system.

## Local verification

```bash
flutter pub get
flutter gen-l10n
flutter analyze
flutter test
flutter build apk --debug
```

Android `minSdk` is 23 because the selected secure-storage implementation uses
the modern Android cryptography path. Never place Panel refresh tokens or full
API keys in SQLite, logs or crash reports.
