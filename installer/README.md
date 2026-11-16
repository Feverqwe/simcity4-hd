# SimCity 4 UHD Setup

The installer is a separate Go module and Qt Widgets project. It packages the
already built Windows game builder, the Qt frontend, and its runtime in one
executable. Use Windows x64 with Go, CMake, 7-Zip, and a Qt 6 kit for MinGW.
Put the matching MinGW tools and the Qt kit's `bin` directory on PATH.
Set `QT_ROOT_DIR` to the Qt kit directory. The [Windows build workflow](../.github/workflows/windows.yml)
shows the release toolchain.

From the root of the public checkout:

```powershell
$env:CGO_ENABLED = '1'
$env:CC = 'gcc'
go build -ldflags '-extldflags=-static' -o dist/simcity4-uhd-windows-amd64.exe .
if ($LASTEXITCODE -ne 0) { throw 'Could not build the Windows backend.' }
./installer/package.ps1 -Backend dist/simcity4-uhd-windows-amd64.exe -Output dist/SimCity4UHD-Setup.exe
```

Use a new output filename when rebuilding the installer. To inspect the
embedded files, run the packaged EXE with `--extract-to DIRECTORY` and a
new directory. The Windows builder is extracted as `app.exe`.

The portable installer includes the SimCity 4 UHD Russian translation and Cyrillic fonts.
To package a different translation revision, unpack the bundled language pack,
replace its locale file, and pass that folder to `package.ps1 -RussianSource`.

For repeated local builds, put `ccache` on PATH and set
`$env:CMAKE_CXX_COMPILER_LAUNCHER = 'ccache'` before running `package.ps1`.

For reuse in another product, see the [Qt frontend contract](qt/README.md).

## Experimental macOS setup

GitHub releases include a ZIP for Apple silicon. To build it locally from
the public checkout, install Go, CMake, Xcode Command Line Tools and Qt 6
with `macdeployqt`, then run `./installer/package-macos.sh`.

The macOS setup app patches the original Aspyr Steam app (version 1.2.1,
build 185949). Choose Interface, World, or both. On Apple silicon with
macOS 14 or later, setup downloads the upscaling tools as needed and builds
missing textures on the GPU. Use **More actions → Import texture images**
to load an Aspyr texture archive, or choose an existing texture cache under
**Advanced options**.
You can also select prepared macOS texture builds from the
[builder CLI](../docs/advanced.md#test-on-macos). Use the same texture scale
for both components when installing them together.
Under **Advanced options**, set the startup resolution, skip the intro, or
show Aspyr's FPS counter. Setup reads the saved resolution when you reopen
**Configure**.
Setup keeps its session logs in `~/Library/Logs/SimCity4HD`.

Close the game and wait for any Steam updates to finish before applying
changes. After setup, launch the game through Steam with your usual profile.
Reopen setup to configure the mod or use **More actions → Remove mod** to
restore the original app. Keep the original backup next to the game until
you remove the mod.
Setup can reuse the installed interface when its options stay the same,
even if you moved the prepared build folder.
