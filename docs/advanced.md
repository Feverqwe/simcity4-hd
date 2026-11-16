# Build and test SimCity 4 UHD

The Windows builder creates the mod from your local game installation. You supply the game, renderer, and HAT checkpoint.

## Build on Windows

### Requirements

- **SimCity 4 Deluxe on Steam**, build 4644414, with the original Windows executable version 1.1.641.0. The builder checks the executable fingerprint. The reference installation used the English locale.
- **64-bit Python 3.12 for Windows** and an NVIDIA GPU that can run the model in bfloat16 through PyTorch CUDA.
- Go for building the utility from source, and free disk space for the Python environment, texture cache, built packages, and a separate test profile.
- A renderer capable of native 3840 × 2160. For SCD3D11, download [v0.5.0](https://github.com/caspervg/scd3d11/releases/tag/v0.5.0).

### Set up Python on Windows

In PowerShell, create a Python environment for texture upscaling. The builder can use it directly on Windows.

```powershell
$venv = Join-Path $env:USERPROFILE 'sc4-hd-venv'
py -3.12 -m venv $venv
$python = Join-Path $venv 'Scripts\python.exe'
& $python -m pip install torch==2.11.0 torchvision==0.26.0 --index-url https://download.pytorch.org/whl/cu128
& $python -m pip install numpy==2.2.6 Pillow==11.3.0 safetensors==0.5.3 spandrel==0.4.2
& $python -c 'import torch; torch.cuda.init(); print(torch.cuda.get_device_name())'
```

Download `IllustrationJaNai_V3detail.zip` and extract `4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors`. [IllustrationJaNai V3 release](https://github.com/the-database/MangaJaNai/releases/tag/3.0.0)

### Build UI and World on Windows

From the repository root, run `go build -o simcity4-uhd-windows-amd64.exe .`. If you use a prebuilt builder, check that `ui -h` lists `-python` and `-weights`. In the same PowerShell session, go to the directory containing the builder and set the Windows paths below.

```powershell
$builder = '.\simcity4-uhd-windows-amd64.exe'
$game = 'D:\SteamLibrary\steamapps\common\SimCity 4 Deluxe'
$cache = 'C:\SC4Test\Cache'
$ui = 'C:\SC4Test\UI'
$world = 'C:\SC4Test\World'
$inference = @(
    '-python', $python,
    '-weights', 'C:\SC4Test\4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors'
)

& $builder ui -texture-scale 4 @inference $game $cache $ui
& $builder world -texture-scale 4 @inference $game $cache $world
```

Run either command on its own or build both. For ×2 packages, use
`-texture-scale 2` with new output directories and the same cache. The first
build can take several hours. Each output contains `Plugins`. Install the
patched `SimCity4-hd.exe` with the plugins from the same UI build. Rebuild UI
after changing the game's language so tutorial text comes from the correct archive.

### Build with Russian text

Use a [cgo-enabled Windows builder](../installer/README.md). If you built it
using the installer guide, point `$builder` at the executable in `dist`.
From the source checkout, unpack the translation into a new directory:

```powershell
& $builder russian-pack unpack .\installer\resources\russian.sc4pack .\russian-source
```

Add `-russian .\russian-source` to the `ui` command.
Use the generated executable, `Plugins`, and `Fonts` together in the isolated
game copy. Back up its original `Fonts` directory before copying the Cyrillic
fonts. To return to the original language, restore the original fonts and switch
to a UI build made without `-russian`.

### Install in an isolated profile

Create the parent directory first and choose new paths for the game copy and
profile. The profile path must not contain spaces. This PowerShell example
copies both builds and the stock regions. For a single build, list just its
`Plugins` directory. For World on its own, skip replacing the game executable.

```powershell
$testGame = 'C:\SC4Test\Game'
$profile = 'C:\SC4Test\Profile'
$renderer = 'C:\SC4Test\Renderer'
$template = $game

foreach ($path in @($testGame, $profile)) {
    if (Test-Path -LiteralPath $path) { throw "Choose a new directory: $path" }
}
$packages = @(Get-ChildItem -LiteralPath @("$ui\Plugins", "$world\Plugins") -Filter '*.dat' -File -ErrorAction Stop)
if ($packages.Count -eq 0) { throw 'No DAT packages found.' }
if ($packages | Group-Object Name | Where-Object Count -gt 1) { throw 'Duplicate DAT filenames.' }
$datPaths = @($packages.FullName)
& $builder verify @datPaths
if ($LASTEXITCODE -ne 0) { throw 'Could not verify the DAT packages.' }
Copy-Item -LiteralPath $game -Destination $testGame -Recurse -ErrorAction Stop
Copy-Item -LiteralPath "$ui\SimCity4-hd.exe" -Destination "$testGame\Apps\SimCity 4.exe" -Force -ErrorAction Stop
New-Item -ItemType Directory -Path "$profile\Plugins" -Force -ErrorAction Stop | Out-Null
Copy-Item -LiteralPath "$template\Regions" -Destination $profile -Recurse -ErrorAction Stop
if (Test-Path -LiteralPath "$template\SimCity 4.cfg") {
    Copy-Item -LiteralPath "$template\SimCity 4.cfg" -Destination $profile -ErrorAction Stop
}
Copy-Item -LiteralPath $datPaths -Destination "$profile\Plugins" -ErrorAction Stop
Copy-Item -LiteralPath @("$renderer\SCD3D11.dll", "$renderer\SCD3D11.ini") `
    -Destination "$profile\Plugins" -ErrorAction Stop
```

To use an existing region, set `$template` to the profile that contains its `Regions` folder.

Preview the launch arguments with `& $builder launch $testGame $profile`.
With Steam open, use `& $builder launch -start $testGame $profile` to start
the isolated game at 3840 × 2160. To return to stock, launch the original
Steam installation with your regular profile.

## Test on macOS

The experimental macOS build targets the English Steam release of SimCity 4
Deluxe by Aspyr, version 1.2.1 (185949). It runs under Rosetta on Apple silicon.
Use the original `.app` as the game path for `world` and `ui`. Those commands
can also build the DATs on a separate machine using the shared texture cache.
On Apple silicon with macOS 14 or later, pass `-python` and `-weights` to
generate missing textures with PyTorch on Metal. Use native arm64 Python
with PyTorch, torchvision, NumPy, Pillow, safetensors, and spandrel, and
the same HAT checkpoint as on Windows.

On Mac, run `go build -o simcity4-uhd .` from the checkout. In a terminal,
copy the original Steam app (`$stock`) to a new `.app` path (`$copy`) outside
the original, then install the prepared builds into that copy:

```sh
test ! -e "$copy" && test ! -L "$copy" && ditto "$stock" "$copy" &&
    ./simcity4-uhd setup install -game "$copy" -ui "$ui" -world "$world"
```

Use matching texture scales. For World only, omit `-ui "$ui"`. For Interface
only, omit `-world "$world"`.
For Russian, build with `CGO_ENABLED=1 go build -o simcity4-uhd .` and unpack
the translation from the checkout with
`./simcity4-uhd russian-pack unpack installer/resources/russian.sc4pack russian-source`.
Pass `-russian russian-source` when building the UI. Setup installs the
matching fonts into the copy. Use the default UI build to play in English.

Pass `-region-map-2x` to double the region map and `-loading-preview-2x`
to double the city loading preview when building the UI. Pass
`-hd-cursors` for larger cursors and use `-cursor-scale 4` for the largest
size. The macOS build uses Aspyr's OpenGL renderer.
Aspyr may play its startup movies before applying the launch options.

With Steam open, copy the original `Contents/GameData/Regions` directory
into a separate profile without spaces in its path and create an empty
`Plugins` folder alongside it. Use `./simcity4-uhd launch "$copy" "$profile"`
to preview the launch parameters and add `-start` before the paths to launch.
Use `-start -foreground` to keep the session attached and see the game's output.
Fullscreen uses the display's backing resolution by default. Use
`-resolution WIDTHxHEIGHT` to override it for a diagnostic launch.
With the UHD interface enabled, choose the resolution and window mode in
Graphics Options and select **Accept** to apply them in the current session.
Borderless fullscreen uses the desktop resolution.
In windowed mode, drag a window edge to resize the game on the fly. The
game scales the frame to fit the window at the selected render resolution.

## Restore the Windows Steam game

Close the game. For a setup-managed installation, use **Remove mod** in
the installer to restore the original executable, fonts and graphics settings
affected by setup.

For a manual installation, verify the game files in Steam or restore the
original executable and any fonts you replaced from your backups. Remove
the mod DATs, launcher and renderer files you copied into the game folder.
