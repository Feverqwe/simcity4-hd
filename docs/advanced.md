# Build and test SimCity 4 UHD


The Windows builder creates the mod from your local game installation. You supply the game, renderer, and HAT checkpoint.

### Requirements

- **SimCity 4 Deluxe on Steam**, build 4644414, with the original Windows executable version 1.1.641.0. The builder checks the executable fingerprint. The reference installation used the English locale.
- **64-bit Python 3.12 for Windows** and an NVIDIA GPU that can run the model in bfloat16 through PyTorch CUDA.
- Free disk space for the Python environment, ×4 texture cache, built packages, and a separate test profile.
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

From a source checkout, run `go build -o simcity4-uhd-windows-amd64.exe .`. If you use a prebuilt builder, check that `ui -h` lists `-python` and `-weights`. In the same PowerShell session, go to the directory containing the builder and set the Windows paths below.

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
& $builder world @inference $game $cache $world
```

Run either command on its own or build both. Each build needs a new output directory. The first build can take several hours. Keep the cache to reuse completed textures. Each output contains `Plugins`. The UI output also includes a patched `SimCity4-hd.exe`. Install it with the `Plugins` from the same UI build. Rebuild UI after changing the game's language so tutorial text comes from the correct archive.

### Build with Russian text

Unpack the Russian resources with `simcity4-uhd russian-pack unpack
installer/resources/russian.sc4pack russian-source`, then add
`-russian russian-source` to the `ui` command.
Use the generated executable, `Plugins`, and `Fonts` together in the isolated
game copy. Back up its original `Fonts` directory before copying the Cyrillic
fonts. To return to the original language, restore the original fonts and switch
to a UI build made without `-russian`.

### Install in an isolated profile

Copy the game into a separate test directory and put the patched UI executable in that copy. Use a fresh profile path without spaces. The example stages both builds and copies the stock regions. You can pass one build's `Plugins` directory to stage just that build. For World on its own, keep the game executable from the copied installation.

```powershell
$testGame = 'C:\SC4Test\Game'
$profile = 'C:\SC4Test\Profile'
$renderer = 'C:\SC4Test\Renderer'

if (Test-Path -LiteralPath $testGame) { throw 'Choose a fresh test game directory.' }
Copy-Item -LiteralPath $game -Destination $testGame -Recurse
Copy-Item -LiteralPath "$ui\SimCity4-hd.exe" -Destination "$testGame\Apps\SimCity 4.exe" -Force
& $builder stage-profile -renderer "$renderer\SCD3D11.dll" -renderer "$renderer\SCD3D11.ini" `
    $game $profile "$ui\Plugins" "$world\Plugins"
```

To use an existing region, substitute a profile template containing its `Regions` folder for the first argument to `stage-profile`.

Preview the launch arguments with `& $builder launch $testGame $profile`. When ready to start the isolated game at 3840 × 2160, use `& $builder launch -start $testGame $profile`. To return to stock, launch the original Steam installation with your regular profile.


Build the portable Windows utility with the [installer project](../installer/README.md).

## Restore the Steam game

Close the game. If setup patched the executable, restore
`Apps/SimCity 4.stock.exe` to `Apps/SimCity 4.exe` or verify the game files in
Steam. Move `Plugins/SimCity4HD` out of the game folder. Remove
`SCD3D11.dll`, `SCD3D11.ini`, `SC4GraphicsOptions.dll`, and
`SC4GraphicsOptions.ini` from `Plugins` if they were added for this mod.
