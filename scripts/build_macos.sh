#!/usr/bin/env bash
# 构建 macOS Universal（arm64 + amd64）.app 并打包为 zip。
# 前置：macOS + Xcode CLT（cgo/clang、sips、iconutil、lipo、codesign、ditto）+ Go。
# 产出：dist/ 下标准版与客户版各一个 zip（自动更新用）+ 一个 dmg（手动安装用）
set -euo pipefail
set -x # CI 日志里保留每条命令，便于无 shell 环境排查
cd "$(dirname "$0")/.."

APP_NAME="Devin_hub"
BUNDLE_ID="com.bailan.devin-relay"
EXE="bailan-relay"
# 版本号：优先取 CI 传入的 APP_VERSION（tag 名 v1.2.4 或手动输入），否则沿用源码里的默认值。
VERSION="${APP_VERSION:-}"
VERSION="${VERSION#v}"
if [ -z "$VERSION" ]; then
  VERSION="$(sed -n 's/^var AppVersion = "\(.*\)"$/\1/p' internal/config/config.go)"
fi
if ! [[ "$VERSION" =~ ^[0-9]+(\.[0-9]+){0,3}$ ]]; then
  echo "非法版本号：'$VERSION'（格式如 1.2.4）" >&2
  exit 1
fi
VERSION_LDFLAGS="-X bailan-relay/internal/config.AppVersion=$VERSION"
DIST="dist"
export MACOSX_DEPLOYMENT_TARGET=12.0

rm -rf build "$DIST"
mkdir -p build "$DIST"

# icon.ico -> PNG -> 全尺寸 iconset -> AppIcon.icns
python3 scripts/ico_to_png.py icon.ico build/icon_256.png
ICONSET="build/Icon.iconset"
mkdir -p build/icons "$ICONSET"
for size in 16 32 64 128 256 512 1024; do
  sips -z "$size" "$size" build/icon_256.png --out "build/icons/$size.png" >/dev/null
done
# iconset 只放 Apple 规定的文件名，多余条目会让 iconutil 直接报错。
cp build/icons/16.png   "$ICONSET/icon_16x16.png"
cp build/icons/32.png   "$ICONSET/icon_16x16@2x.png"
cp build/icons/32.png   "$ICONSET/icon_32x32.png"
cp build/icons/64.png   "$ICONSET/icon_32x32@2x.png"
cp build/icons/128.png  "$ICONSET/icon_128x128.png"
cp build/icons/256.png  "$ICONSET/icon_128x128@2x.png"
cp build/icons/256.png  "$ICONSET/icon_256x256.png"
cp build/icons/512.png  "$ICONSET/icon_256x256@2x.png"
cp build/icons/512.png  "$ICONSET/icon_512x512.png"
cp build/icons/1024.png "$ICONSET/icon_512x512@2x.png"
iconutil -c icns "$ICONSET" -o build/AppIcon.icns

# 按 edition 构建两个 Universal 二进制：单架构 cgo 交叉编译后 lipo 合并。
# 注意 clang 的架构名与 GOARCH 不同：amd64 -> x86_64。
build_edition() { # $1=产物名 $2=ldflags 追加
  for arch in arm64 amd64; do
    case "$arch" in
      arm64) clang_arch=arm64 ;;
      amd64) clang_arch=x86_64 ;;
    esac
    mkdir -p "build/$arch"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" \
      CC="clang -arch $clang_arch" CXX="clang++ -arch $clang_arch" \
      go build -trimpath -ldflags "-s -w $VERSION_LDFLAGS $2" -o "build/$arch/$EXE" .
  done
  lipo "build/arm64/$EXE" "build/amd64/$EXE" -create -output "$1"
  lipo -archs "$1"
}

package_app() { # $1=二进制 $2=输出 zip $3=输出 dmg
  APP="build/$APP_NAME.app"
  rm -rf "$APP"
  mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
  cp "$1" "$APP/Contents/MacOS/$EXE"
  cp build/AppIcon.icns "$APP/Contents/Resources/AppIcon.icns"
  printf 'APPL????' > "$APP/Contents/PkgInfo"
  cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key><string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
    <key>CFBundleExecutable</key><string>${EXE}</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleIconFile</key><string>AppIcon</string>
    <key>CFBundleShortVersionString</key><string>${VERSION}</string>
    <key>CFBundleVersion</key><string>${VERSION}</string>
    <key>LSMinimumSystemVersion</key><string>12.0</string>
    <key>NSHighResolutionCapable</key><true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key><true/>
</dict>
</plist>
PLIST
  # 无开发者证书：做 ad-hoc 签名（arm64 必需；不公证，首次打开需右键“打开”）。
  codesign --force --sign - "$APP"
  # zip 供自动更新下载解包；dmg 供用户手动安装（拖到“应用程序”）。
  ditto -c -k --sequesterRsrc --keepParent "$APP" "$2"
  echo "packaged: $2"
  make_dmg "$APP" "$3"
}

make_dmg() { # $1=.app 路径 $2=输出 dmg
  local root="build/dmg-root"
  rm -rf "$root"
  mkdir -p "$root"
  ditto "$1" "$root/$(basename "$1")"
  ln -s /Applications "$root/Applications"
  # GitHub 的 macOS runner 上 hdiutil 偶发 “Resource busy”，重试几次。
  local i
  for i in 1 2 3 4 5; do
    if hdiutil create -volname "$APP_NAME" -srcfolder "$root" -ov -format UDZO "$2"; then
      echo "packaged: $2"
      return 0
    fi
    sleep 5
  done
  echo "DMG 创建失败：$2" >&2
  return 1
}

build_edition "build/universal-standard" ""
package_app "build/universal-standard" "$DIST/bailan-relay-macOS-universal.zip" "$DIST/bailan-relay-macOS-universal.dmg"

build_edition "build/universal-customer" "-X bailan-relay/internal/config.Edition=customer"
package_app "build/universal-customer" "$DIST/bailan-relay-customer-macOS-universal.zip" "$DIST/bailan-relay-customer-macOS-universal.dmg"

ls -lh "$DIST"
