# SimCity 4 UHD Setup

The installer is a separate Go module and Qt Widgets project. It packages the
already built Windows game builder, the Qt frontend, and its runtime in one
executable. Use Windows x64 with Go, CMake, 7-Zip, and a Qt kit for MinGW.
Put the matching MinGW tools and the Qt kit's `bin` directory on PATH.
Set `QT_ROOT_DIR` to the Qt kit directory. The [Windows build workflow](../.github/workflows/windows.yml)
shows the release toolchain.

From the root of the public checkout:

```powershell
$env:CGO_ENABLED = '1'
$env:CC = 'gcc'
go build -ldflags '-extldflags=-static' -o dist/simcity4-uhd-windows-amd64.exe .
./installer/package.ps1 -Backend dist/simcity4-uhd-windows-amd64.exe -Output dist/SimCity4UHD-Setup.exe
```

Use a new output filename when rebuilding the installer. To inspect the
embedded files, run the packaged EXE with `--extract-to DIRECTORY` and a
new directory. The Windows builder is extracted as `app.exe`.

The portable installer includes the SimCity 4 UHD Russian translation and Cyrillic fonts.
To package a different translation revision, unpack the bundled language pack,
replace its locale file, and pass that folder to `package.ps1 -RussianSource`.

The UHD interface option also builds the updated advisors from the installed
Steam game assets. The builder includes the data it needs for this step.

For repeated local builds, put `ccache` on PATH and set
`$env:CMAKE_CXX_COMPILER_LAUNCHER = 'ccache'` before running `package.ps1`.

GitHub Actions builds the same installer using the verified Windows builder.
The packaged launcher makes its directory available as `${SETUP_EXE_DIR}`
for path defaults in the product manifest.
For reuse in another product, see the [Qt frontend contract](qt/README.md).
