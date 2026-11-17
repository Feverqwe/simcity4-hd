#!/bin/bash
set -euo pipefail

if [[ ${1:-} == --help || ${1:-} == -h ]]; then
    cat <<'HELP'
Usage: installer/package-macos.sh

Build a ZIP with the macOS setup app and its Qt runtime.
Requires macOS, Go, CMake, Xcode Command Line Tools, and Qt 6.5 or later.

QT_ROOT_DIR      Native Qt kit (default: Homebrew qtbase, or macdeployqt on PATH)
SC4_SETUP_BUILD_DIR  Build directory (default: build/installer-macos)
SC4_SETUP_VERSION  Release tag such as v1.2.3 (default: dev)
HELP
    exit 0
fi
if (( $# != 0 )) || [[ $(uname -s) != Darwin ]]; then
    printf 'Usage: %s (on macOS)\n' "$0" >&2
    exit 2
fi

version=${SC4_SETUP_VERSION:-dev}
if [[ $version != dev && ! $version =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
    printf 'SC4_SETUP_VERSION must be a release tag such as v1.2.3.\n' >&2
    exit 2
fi

public=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
arch=$(uname -m)
build="${SC4_SETUP_BUILD_DIR:-$public/build/installer-macos}/$arch"
mkdir -p "$build"
build=$(cd -- "$build" && pwd -P)
stage="$build/stage"
if ! mkdir "$stage"; then
    printf 'The macOS installer build is already using %s\n' "$build" >&2
    exit 1
fi
trap 'rm -rf -- "$stage"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

qt="${QT_ROOT_DIR:-}"
if [[ -z $qt ]] && command -v brew >/dev/null; then
    qt=$(brew --prefix qtbase 2>/dev/null || true)
    [[ -x "$qt/bin/macdeployqt" ]] || qt=''
fi
if [[ -n $qt ]]; then
    deploy="$qt/bin/macdeployqt"
    qt_args=(-DCMAKE_PREFIX_PATH="$qt")
else
    deploy=$(command -v macdeployqt || true)
    qt_args=()
    if [[ -n $deploy && -x ${deploy%/*}/qmake ]]; then
        qt_args=(-DCMAKE_PREFIX_PATH="$("${deploy%/*}/qmake" -query QT_INSTALL_PREFIX)")
    fi
fi
if [[ ! -x $deploy ]]; then
    printf 'Set QT_ROOT_DIR to a Qt kit containing macdeployqt.\n' >&2
    exit 1
fi

# Some Command Line Tools installations have libc++ only in the SDK.
if [[ -z ${CXX:-} ]] && ! printf '#include <type_traits>\n' | xcrun clang++ -x c++ -fsyntax-only - 2>/dev/null; then
    sdk=$(xcrun --show-sdk-path)
    if [[ -f "$sdk/usr/include/c++/v1/type_traits" ]]; then
        export CPLUS_INCLUDE_PATH="$sdk/usr/include/c++/v1${CPLUS_INCLUDE_PATH:+:$CPLUS_INCLUDE_PATH}"
    fi
fi
case "$arch" in arm64) goarch=arm64 ;; x86_64) goarch=amd64 ;; *) exit 2 ;; esac
cmake -S "$public/installer/qt" -B "$build/qt" "${qt_args[@]+${qt_args[@]}}" \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_OSX_ARCHITECTURES="$arch"
cmake --build "$build/qt" --config Release --parallel
cmake --install "$build/qt" --config Release --prefix "$stage"

app="$stage/SimCity 4 UHD Setup.app"
mv "$stage/qt/Setup-UI.app" "$app"
mkdir -p "$app/Contents/Resources/Russian"
cp "$public/installer/setup.json" "$public/assets/screenshot.jpg" "$app/Contents/Resources/"
/usr/bin/sips -s format icns "$public/installer/setup.ico" --out "$app/Contents/Resources/setup.icns" >/dev/null
cp "$public/installer/resources/russian.sc4pack" "$public/installer/resources/FONT-LICENSES.txt" "$app/Contents/Resources/Russian/"
(cd "$public" && GOOS=darwin GOARCH="$goarch" CGO_ENABLED=1 go build -trimpath -ldflags "-X main.version=$version" -o "$app/Contents/MacOS/simcity4-uhd" .)
"$deploy" "$app" -no-strip
/usr/bin/codesign --verify --strict --deep "$app"
[[ $("$app/Contents/MacOS/simcity4-uhd" --version) == "$version" ]]
HOME="$stage" "$app/Contents/MacOS/simcity4-uhd" setup inspect >/dev/null

archive="SimCity4UHD-Setup-macos-$arch.zip"
/usr/bin/ditto -c -k --sequesterRsrc --keepParent "$app" "$stage/$archive"
mv -f "$stage/$archive" "$build/$archive"
printf '%s\n' "$build/$archive"
