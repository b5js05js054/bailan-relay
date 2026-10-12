#!/usr/bin/env bash
# 构建 macOS Universal（arm64 + amd64）.app 并打包为 zip。
# 前置：macOS + Xcode CLT（cgo/clang、sips、iconutil、lipo、codesign、ditto）+ Go。
# 产出：dist/bailan-relay-macOS-universal.zip 与 dist/bailan-relay-customer-macOS-universal.zip
set -euo pipefail
cd "$(dirname "$0")/.."

APP_NAME="Devin_hub"
BUNDLE_ID="com.bailan.devin-relay"
EXE="bailan-relay"
VERSION="1.0.0"
DIST="dist"
MACOSX_DEPLOYMENT_TARGET=12.0

rm -rf build "$DIST"
mkdir -p build "$DIST"

# icon.ico -> PNG -> 全尺寸 iconset -> AppIcon.icns
python3 scripts/ico_to_png.py icon.ico build/icon_256.png
ICONSET="build/Icon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 64 128 256 512 1024; do
  sips -z "$size" "$size" build/icon_256.png --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
done
cp "$ICONSET/icon_32x32.png" "$ICONSET/icon_16x16@2x.png"
cp "$ICONSET/icon_64x64.png" "$ICONSET/icon_32x32@2x.png"
cp "$ICONSET/icon_256x256.png" "$ICONSET/icon_128x128@2x.png"
cp "$ICONSET/icon_512x512.png" "$ICONSET/icon_256x256@2x.png"
cp "$ICONSET/icon_1024x1024.png" "$ICONSET/icon_512x512@2x.png"
iconutil -c icns "$ICONSET" -o build/AppIcon.icns

# 按 edition 构建两个 Universal 二进制：单架构 cgo 交叉编译后 lipo 合并。
build_edition() { # $1=标准版产物名 $2=ldflags 追加
  for arch in arm64 amd64; do
    mkdir -p "build/$arch"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" \
      CC="clang -arch $arch" CXX="clang++ -arch $arch" \
      go build -trimpath -ldflags "-s -w $2" -o "build/$arch/$EXE" .
  done
  lipo "build/arm64/$EXE" "build/amd64/$EXE" -create -output "$1"
}

package_app() { # $1=二进制 $2=输出 zip
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
  ditto -c -k --sequesterRsrc --keepParent "$APP" "$2"
  echo "packaged: $2"
}

build_edition "build/universal-standard" ""
package_app "build/universal-standard" "$DIST/bailan-relay-macOS-universal.zip"

build_edition "build/universal-customer" "-X bailan-relay/internal/config.Edition=customer"
package_app "build/universal-customer" "$DIST/bailan-relay-customer-macOS-universal.zip"

ls -lh "$DIST"
