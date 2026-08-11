# 2026-08-11 构建产物

本页记录移动端本地聊天历史交付对应的可安装产物、来源和验证结果。功能提交为
`d51f361b9 feat(mobile): persist local chat history`。

## CI 原生构建

GitHub Actions run `31467467284` 全部通过，包括 Flutter analyze/test、Android debug APK、
Ubuntu Debian 包、Windows NSIS 安装包、PC Web、Cargo check 和 Rust tests。产物已下载到：

```text
/home/test/Downloads/sub2api-deploy/mobile-codex/artifacts/ci-31467467284/
├── android/app-debug.apk
├── ubuntu/Codex PC Companion_0.1.0_amd64.deb
└── windows/Codex PC Companion_0.1.0_x64-setup.exe
```

| 平台 | 文件 | SHA-256 | 验证 |
|---|---|---|---|
| Android | `android/app-debug.apk` | `173f02bab1512bfb75113c17a477ae46e0aeb2c93c82f3507f7924f7137d78c1` | ZIP 完整性通过 |
| Ubuntu amd64 | `ubuntu/Codex PC Companion_0.1.0_amd64.deb` | `fcd727f8dca004bb2f87cfd4653f557f0ec8caedadaa45ce575fda4d59e02619` | `dpkg-deb --info` 通过 |
| Windows x64 | `windows/Codex PC Companion_0.1.0_x64-setup.exe` | `ced311fd34cecfbecf8468bc328e0a0b8fe6379d9d6b4f5cd46d9fa723144325` | PE32/NSIS 格式识别通过 |

Debian 包名为 `codex-pc-companion`、版本 `0.1.0`、架构 `amd64`，运行依赖为
`libwebkit2gtk-4.1-0` 和 `libgtk-3-0`。

## 指定服务器复现构建

服务器源码目录为：

```text
/home/test/Downloads/sub2api-deploy/mobile-codex/clients/mobile
/home/test/Downloads/sub2api-deploy/mobile-codex/clients/codex-pc
```

Android 构建使用校验过的官方 Flutter 3.44.9 / Dart 3.12.2 SDK，以及仓库中的
`clients/mobile/Dockerfile.android-builder`（Java 17、Android SDK 36、Build Tools 36）。执行顺序为：

```text
flutter pub get
flutter gen-l10n
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test
flutter build apk --debug
```

服务器复现构建结果：格式检查 42 个文件无变化、Flutter analyze 无问题、15 项测试全部
通过，`assembleDebug` 成功。最终产物为：

```text
/home/test/Downloads/sub2api-deploy/mobile-codex/artifacts/server-build/android/sub2api-mobile-server-debug.apk
```

- 大小：`165181486` bytes
- SHA-256：`4e355c90b03e889a714f47237c00f1035801dbf8d685508a49074a29e8663a31`
- 验证：`unzip -tqq` 通过，`file` 识别为 ZIP/APK

服务器位于中国网络环境，因此仅在构建缓存层使用 Flutter 中国存储镜像和 Maven 镜像，
同时保留官方仓库回退；App 运行时代码及网络端点不受影响。SQLite 3.5.1 的三个 Android
原生库均按包内固定 SHA-256 校验后进入 Native Assets 构建缓存。

## 发布边界

- Android 产物是 debug APK，可用于安装和联调，不是应用商店 release 包。
- 尚未提供 Android release keystore、Windows 代码签名证书；不得把未签名产物冒充正式发布包。
- Windows 安装包由 GitHub `windows-latest` 原生构建，Ubuntu 包由 Ubuntu runner 原生构建，
  不是跨平台伪造文件。
