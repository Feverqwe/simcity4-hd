# SimCity 4 UHD Setup

The installer is a separate Go module and Qt Widgets project. It packages the
already built Windows game builder, the Qt frontend, and its runtime in one
executable. Use Windows x64 with Go, CMake, Qt 6.12.0 for MinGW, and the matching
MinGW compiler on PATH. Set `QT_ROOT_DIR` to the Qt kit directory.

From the root of the public checkout:

```powershell
go build -o dist/simcity4-uhd-windows-amd64.exe .
./installer/package.ps1 -Backend dist/simcity4-uhd-windows-amd64.exe -Output dist/SimCity4UHD-Setup.exe
```

For repeated local builds, put `ccache` on PATH and set
`$env:CMAKE_CXX_COMPILER_LAUNCHER = 'ccache'` before running `package.ps1`.

GitHub Actions builds the same installer using the verified Windows builder.
The packaged launcher makes its directory available as `${SETUP_EXE_DIR}`
for path defaults in the product manifest.
For reuse in another product, see the [Qt frontend contract](qt/README.md).
