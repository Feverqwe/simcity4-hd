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
profile. The profile path must not contain spaces. This example stages both
builds and copies the stock regions. For a single build, pass just its
`Plugins` directory. For World on its own, skip replacing the game executable.

```powershell
$testGame = 'C:\SC4Test\Game'
$profile = 'C:\SC4Test\Profile'
$renderer = 'C:\SC4Test\Renderer'

foreach ($path in @($testGame, $profile)) {
    if (Test-Path -LiteralPath $path) { throw "Choose a new directory: $path" }
}
Copy-Item -LiteralPath $game -Destination $testGame -Recurse -ErrorAction Stop
Copy-Item -LiteralPath "$ui\SimCity4-hd.exe" -Destination "$testGame\Apps\SimCity 4.exe" -Force -ErrorAction Stop
& $builder stage-profile -renderer "$renderer\SCD3D11.dll" -renderer "$renderer\SCD3D11.ini" `
    $game $profile "$ui\Plugins" "$world\Plugins"
if ($LASTEXITCODE -ne 0) { throw 'Could not stage the test profile.' }
```

To use an existing region, substitute a profile template containing its `Regions` folder for the first argument to `stage-profile`.

Preview the launch arguments with `& $builder launch $testGame $profile`.
With Steam open, use `& $builder launch -start $testGame $profile` to start
the isolated game at 3840 × 2160. To return to stock, launch the original
Steam installation with your regular profile.

## Test on macOS

The experimental macOS build targets the English Steam release of SimCity 4
Deluxe by Aspyr, version 1.2.1 (185949). It runs under Rosetta on Apple silicon.
Use the original `.app` as the game path for `world` and `ui`. Those commands
can also build the DATs on a separate machine using the shared texture cache.

On Mac, run `go build -o simcity4-uhd .` from the checkout and stage a separate bundle. To use UHD
world textures with the original interface, run
`./simcity4-uhd setup stage-world "$stock" "$world" "$copy"`.
To include the enlarged interface, use
`./simcity4-uhd setup stage "$stock" "$ui" "$copy" "$world"`. Omit `"$world"`
for an interface-only build.
Use a fresh `.app` path for the copy and matching texture scales for UI and World.
For Russian, build with `CGO_ENABLED=1 go build -o simcity4-uhd .` and unpack
the translation from the checkout with
`./simcity4-uhd russian-pack unpack installer/resources/russian.sc4pack russian-source`.
Pass `-russian russian-source` when building the UI. Staging installs the
matching fonts into the new bundle. Stage a separate copy with the default UI
build to play in English.

Larger cursors, the doubled region map, and the doubled city loading preview
are currently Windows options. The macOS build uses Aspyr's OpenGL renderer.
Aspyr may play its startup movies before applying the launch options.

With Steam open, copy the original `Contents/GameData/Regions` directory
into a separate profile without spaces in its path and create an empty
`Plugins` folder alongside it. Use `./simcity4-uhd launch "$copy" "$profile"`
to preview the launch parameters and add `-start` before the paths to launch.
Use `-start -foreground` to keep the session attached and see the game's output.
Launch the staged copy through this command to use the separate profile.
For the doubled interface, use `-resolution WIDTHxHEIGHT` to request the
display's backing resolution. On Retina, compare the menu size at that
resolution with the original game on the same display.

## Restore the Windows Steam game

Close the game. For a setup-managed installation, use **Remove mod** in
the installer to restore the original executable, fonts and graphics settings
affected by setup.

For a manual installation, verify the game files in Steam or restore the
original executable and any fonts you replaced from your backups. Remove
the mod DATs, launcher and renderer files you copied into the game folder.
