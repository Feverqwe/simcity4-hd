# SimCity 4 HD

An HD mod preview for **SimCity 4 Deluxe on Windows**, with a larger interface and ×4 upscaled UI and world textures for native 4K.

![SimCity 4 HD gameplay at 3840 × 2160](assets/screenshot.jpg)

## Download Ready-to-Copy for Windows

Download the prebuilt `.7z` archive from either mirror. It includes the HD texture packs, a patched game executable, the SCD3D11 renderer, and a 4K launcher.

- [OneDrive](https://1drv.ms/f/c/93e54109264f3ede/IgCk1v2pSLulRb8F38sXonzfATgkeaHfZUgB9Yx_hOz1G_E)
- [Yandex Disk](https://disk.yandex.ru/d/ybsDYO0X-H8K8w)

The package requires the Steam version of SimCity 4 Deluxe (build 4644414). Open `README-HD.txt` in the archive for instructions on backing up the original executable, extracting the files into your game folder, and launching at 3840 × 2160.

## Build from source

The Windows builder creates the mod from your local game installation. You supply the game, renderer, and HAT checkpoint.

### Requirements

- **SimCity 4 Deluxe on Steam**, build 4644414, with the original Windows executable version 1.1.641.0. The builder checks the executable fingerprint. The reference installation used the English locale.
- **64-bit Python 3.12 for Windows** and an NVIDIA GPU with CUDA and bfloat16 support.
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
& $python -c 'import torch; assert torch.cuda.is_available() and torch.cuda.is_bf16_supported(including_emulation=False); print(torch.cuda.get_device_name())'
```

Download `IllustrationJaNai_V3detail.zip` and extract `4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors`. [IllustrationJaNai V3 release](https://github.com/the-database/MangaJaNai/releases/tag/3.0.0)

### Build UI and World on Windows

From a source checkout, run `go build -o simcity4-hd-windows-amd64.exe .`. If you use a prebuilt builder, check that `ui -h` lists `-python` and `-weights`. In the same PowerShell session, go to the directory containing the builder and set the Windows paths below.

```powershell
$builder = '.\simcity4-hd-windows-amd64.exe'
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

Run either command on its own or build both. Each build needs a new output directory. The first build can take several hours. Keep the cache to reuse completed textures. Each output contains `Plugins`. The UI output also includes a patched `SimCity4-hd.exe`. Use `ui -texture-scale 2` for the ×2 UI texture variant. Both variants use the same HUD size on screen. Keep each variant in a separate output directory and install its matching executable and Plugins together. Rebuild UI after changing the game's language so tutorial text comes from the correct archive.

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

## Preview status

The ×4 packages are a development preview. The reference build was tested in-game at 3840 × 2160. Minor artifacts can appear at the ends of the City Opinion Polls meters.

The launcher requests 4K by default. For larger cursors, a doubled region map and a doubled city loading preview, add `-hd-cursors -region-map-2x -loading-preview-2x` to the `ui` command. Each option can also be used on its own.
