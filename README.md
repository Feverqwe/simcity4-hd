# SimCity 4 UHD

SimCity 4 UHD brings 4K display support, a larger interface, and sharper artwork to **SimCity 4 Deluxe**. Play your existing cities and regions with your usual Steam profile. You can install the interface, the world textures, or both.

![SimCity 4 UHD gameplay at 3840 × 2160](assets/screenshot.jpg)

## What the mod changes

### Interface

The UHD interface doubles the size of most controls compared with the original game at the same resolution. Both texture options use the same enlarged layout. The ×2 and ×4 settings control the detail of the artwork.

- **City controls.** Larger buttons, icons, and labels in God, Mayor, and My Sims modes, from construction and transport menus to Disasters, simulation speed, and Help. Button states fit their panels, and the animated 3D advisor portraits are sharper.
- **Graphs and Data Views.** Larger plots and labels, thicker graph lines, and clearer legends. Data Views maps show terrain and water beneath the overlays. Mayor Rating arrows and City Opinion Polls meters retain their outlines.
- **Budgets and ordinances.** Larger ledgers, funding sliders, Ordinance details, and Neighbor Deals. Amounts stay readable when values and lists refresh. Numeric fields accept your locale’s decimal separator.
- **Queries, messages, and tutorials.** Building and route queries give more room to text, icons, and meters. News, advisor messages, tutorials, and tooltips wrap within their panels, with scrollbars that fit the larger layout. On Windows, the mod also fixes crashes involving the city HUD, certain menus, and building queries.
- **Region and City Import.** Enable a larger interactive region map and city loading preview. The City Import folder tree uses larger folder and city icons and more readable labels. Larger in-game cursors are also optional.
- **Driving and snapshots.** Free Drive has larger gauges and a larger button to restore the collapsed driving panel. Its viewport artwork is sharper on Windows. The Snapshot tool has larger instructions and a capture frame that follows the selected size.
- **Options.** Audio Options lets you scroll the playlist all the way to the final track with the mouse wheel. Play Options includes a keyboard shortcut reference for city, region, and driving controls.

### World textures

The World component upscales original game artwork at ×2 or ×4. In setup, choose which groups to install.

- **Roads and rail** sharpen the transport network.
- **Terrain** adds detail to the ground as you zoom in.
- **Buildings, props and effects** cover building facades, lot surfaces, station and transit artwork, and animated textures, including lots with multiple animation frames.

### Russian translation

Setup includes a Russian translation and Cyrillic fonts. Long labels fit the game’s buttons, budget tables, and dialogs such as Building Style Control and Select a Bridge. Hover over shortened captions to read the full text where available. The new display settings and keyboard shortcut reference are also translated.

With **Interface** enabled, choose **Advanced options → Game language → Русский** in setup. To switch back, open **Configure** and select **Game default**.

## Supported Steam editions and downloads

| Platform | Required game version | Installer |
| --- | --- | --- |
| Windows x64 | SimCity 4 Deluxe 1.1.641.0 (Steam build 4644414) | [SimCity4UHD-Setup.exe](https://github.com/Feverqwe/simcity4-hd/releases/latest/download/SimCity4UHD-Setup.exe) |
| macOS on Apple silicon | Aspyr SimCity 4 Deluxe 1.2.1 (Steam build 185949), running through Rosetta | [SimCity4UHD-Setup-macos-arm64.zip](https://github.com/Feverqwe/simcity4-hd/releases/latest/download/SimCity4UHD-Setup-macos-arm64.zip) |

With **Interface** installed, open **Graphics Options** to choose the render resolution and switch between windowed, fullscreen, and borderless fullscreen. Borderless fullscreen uses the desktop resolution.

- **Windows** uses Direct3D 11 through SCD3D11. Select **Accept** and restart the game to apply display changes.
- **macOS** uses Aspyr’s OpenGL renderer and applies display changes in the current session. Drag a window edge to resize it while playing. The UHD interface also renders sharper text on Retina displays.

## Install and play

1. Download the installer for your platform and close SimCity 4. Run the EXE on Windows. On macOS, extract the ZIP and open **SimCity 4 UHD Setup**.
2. If setup cannot find your Steam copy, select it under **Advanced options → Game folder**. In **Advanced options** you can also choose components, set the startup resolution, and turn the intro on or off. Setup defaults to ×2 textures.
3. Select **Install**. After setup finishes, select **Play** or launch **SimCity 4 Deluxe** from Steam.

### Texture generation and disk space

Setup prepares textures from your installed copy of the game. It downloads the tools and model when it first needs to generate textures.

- **Windows** requires an NVIDIA GPU with CUDA and bfloat16 support to generate textures. The full ×4 installation takes roughly 20 GiB, with additional space for the cache and downloads. Setup checks available space before building.
- **macOS** uses Metal on Apple silicon and requires macOS 14 or later for texture generation.

An existing cache with all the textures for your selected options lets setup skip GPU upscaling. You can pause a build and resume later using the same cache.

### Reuse textures, change options, or remove the mod

- **Reuse a cache.** Setup stores textures in `SimCity4UHDCache` next to the installer by default. To reuse a cache from another folder, select it under **Advanced options → Texture cache**. Select **Export textures** after installation to save a ZIP, then use **More actions → Import textures** in setup before installing from that archive.
- **Change options.** Close the game, reopen setup, select **Configure**, and choose **Apply changes** when you’re done.
- **Restore the original game.** Close the game and select **More actions → Remove mod**. Setup restores the original game files and keeps the texture cache.

Setup displays errors in its progress panel. Session logs are under `%LOCALAPPDATA%\SimCity4HD\Logs` on Windows and `~/Library/Logs/SimCity4HD` on macOS.

## Further reading

- [Build the mod from source or test in a separate profile](docs/advanced.md)
- [Build the installers](installer/README.md)
- [Discuss the mod on Discord](https://discord.gg/PHmyWuBHj)
