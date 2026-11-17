# SimCity 4 UHD Setup

## Windows setup

The Windows packager bundles the prebuilt game builder, Qt frontend and
runtime in one portable EXE. Use Windows x64 with Go, 7-Zip and a MinGW
Qt kit that meets the [frontend build requirements](qt/README.md). Put
the matching MinGW tools and the Qt kit's `bin` directory on PATH.
Set `QT_ROOT_DIR` to the Qt kit directory. The [release workflow](../.github/workflows/windows.yml)
shows the release toolchain for both platforms.

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
new directory. The Windows builder is extracted as `app.exe`. Open the
packaged setup on Windows to verify that its bundled Qt runtime opens the
window.

The portable installer includes the SimCity 4 UHD Russian translation and Cyrillic fonts.
To package a different translation revision, unpack the bundled language pack,
replace its locale file, and pass that folder to `package.ps1 -RussianSource`.

For repeated local builds, put `ccache` on PATH and set
`$env:CMAKE_CXX_COMPILER_LAUNCHER = 'ccache'` before running `package.ps1`.

For reuse in another product, see the [Qt frontend contract](qt/README.md).

## macOS setup

You can run Setup on either operating system with a copy of the supported
Steam game for the other platform. Select the game folder, Aspyr `.app`,
or game executable in **Advanced options** and click **Install**. The computer running
Setup supplies the GPU for any missing textures.
Use **Export textures** after installation to take the completed textures
to the other computer for future changes.

After installing the Aspyr mod on Windows, copy the patched `.app` and its
adjacent `.sc4-uhd-original` backup folder to the same location on your Mac.
Select the patched game in macOS Setup, click **Configure**, then click
**Apply changes** to sign the app with its existing settings. Keep the
backup next to the app for future configuration or removal.

After installing for the Windows edition on a Mac, use the patched game
folder on Windows. **Automatic** runs the Windows game at the primary
display's resolution in borderless fullscreen.

To build the release ZIP locally, use a native Apple silicon terminal with
Go, Xcode Command Line Tools and a native Qt kit with `macdeployqt`.
Follow the [frontend build requirements](qt/README.md) and run
`./installer/package-macos.sh` from the public checkout. The command prints
the path to the ZIP. Extract it and open the setup app on macOS to check the
bundled runtime.

The macOS setup app patches the original Aspyr Steam app (version 1.2.1,
build 185949). Choose Interface, World, or both. On Apple silicon with
macOS 14 or later, setup downloads the upscaling tools as needed and builds
missing textures on the GPU. Use **More actions → Import texture images**
to load an Aspyr texture archive, or choose an existing texture cache under
**Advanced options**. By default, setup stores textures in a
`SimCity4UHDCache` folder next to the setup app.
Under **Advanced options**, set the startup resolution or skip the intro.
Setup reads the saved resolution when you reopen **Configure**.
Setup keeps its session logs in `~/Library/Logs/SimCity4HD`.

Close the game and wait for any Steam updates to finish before applying
changes. After setup, launch the game through Steam with your usual profile.
Reopen setup to configure the mod or use **More actions → Remove mod** to
restore the original game files. Keep the setup backup next to the game
until you remove the mod.
