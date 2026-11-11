# SimCity 4 HD

A build-from-source HD mod preview for **SimCity 4 Deluxe on Windows**. It builds UI and world texture packs at ×4 from stock game resources and targets a native 4K profile.

The Windows builder creates the mod from your local game installation. You supply the game, renderer, and HAT checkpoint.

## Requirements

- **SimCity 4 Deluxe on Steam**, build 4644414, with the original Windows executable version 1.1.641.0. The builder checks the executable fingerprint. The reference installation used the English locale.
- **WSL 2** with Python 3.12 and a CUDA-capable NVIDIA setup. The reference build used an RTX 5070 Ti and PyTorch 2.11 with CUDA 12.8.
- Free disk space for the WSL environment, inference cache and test profile. Allow disk space for the ×4 texture cache, the built packages, and a separate test profile.
- A renderer capable of native 3840 × 2160. For SCD3D11, download [v0.5.0](https://github.com/caspervg/scd3d11/releases/tag/v0.5.0).

## Set up inference in WSL

Install the Python dependencies in WSL. The Windows builder invokes this interpreter for HAT inference.

```bash
python3 -m venv ~/.venvs/sc4-hd
source ~/.venvs/sc4-hd/bin/activate
python -m pip install torch==2.11.0 torchvision==0.26.0 --index-url https://download.pytorch.org/whl/cu128
python -m pip install numpy==2.2.6 Pillow==11.3.0 safetensors==0.5.3 spandrel==0.4.2
python -c 'import torch; assert torch.cuda.is_available(); print(torch.cuda.get_device_name())'
```

Download `IllustrationJaNai_V3detail.zip` and extract `4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors`. [IllustrationJaNai V3 release](https://github.com/the-database/MangaJaNai/releases/tag/3.0.0)

## Build UI and World on Windows

Download the Windows executable from GitHub Releases, or run `go build -o simcity4-hd-windows-amd64.exe .` from a source checkout. Open PowerShell in the directory containing the builder. Use Windows paths for the game, cache, and output. The Python interpreter and weights use WSL paths.

```powershell
$builder = '.\simcity4-hd-windows-amd64.exe'
$game = 'D:\SteamLibrary\steamapps\common\SimCity 4 Deluxe'
$cache = 'C:\SC4Test\Cache'
$ui = 'C:\SC4Test\UI'
$world = 'C:\SC4Test\World'
$inference = @(
    '-wsl-distro', 'Ubuntu-24.04',
    '-wsl-python', '/home/your-user/.venvs/sc4-hd/bin/python',
    '-wsl-weights', '/path/to/4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors'
)

& $builder ui -texture-scale 4 @inference $game $cache $ui
& $builder world @inference $game $cache $world
```

Run either command on its own or build both. Each build needs a new output directory. The first build can take several hours. Keep the cache to reuse completed textures. Each output contains `Plugins`. The UI output also includes a patched `SimCity4-hd.exe`. Use `ui -texture-scale 2` for the ×2 UI texture variant. Both variants use the same HUD size on screen. Keep each variant in a separate output directory and install its matching executable and Plugins together. Rebuild UI after changing the game's language so tutorial text comes from the correct archive.

## Install in an isolated profile

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
