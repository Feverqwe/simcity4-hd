# Installer agent guide

## Project boundaries

- Keep the installer as a separate subproject. The Qt Widgets frontend in `qt/` communicates with an already built backend executable through the [manifest and process contract](qt/README.md). Put product branding and options in the manifest, and keep Steam discovery, texture processing, and game modifications in the SimCity Go backend under `../internal/sc4/`.
- Keep the Windows deliverable a single portable EXE. The [packaging script](package.ps1) embeds the verified backend and Qt runtime. The Qt SDK, CMake, and MinGW are build-time requirements. Resize the banner for the installer during packaging and keep the source image available for the main README.
- Apply the mod to the existing Steam game directory so the user can play through Steam. Keep the default HD setup and write editable settings where the relevant DLL reads them. Use the game's existing profile and avoid Windows installer registration, persistent services, and installer shortcuts.

## User flow and layout

- Make **Install** the normal path. Detect the game and use sensible defaults. Ask for a required value only when detection fails or validation needs user input. Keep directory overrides, resolution, and grouped checkboxes in **Advanced options**. Keep interface upscaling selectable separately from world textures.
- Inspect the game folder at startup and when the user selects another one. Load the installed options before offering **Configure** or **Remove mod**. Reuse assembled packages and preserve the texture cache across both operations.
- Separate the start, progress, and completion screens. After a successful operation, offer **Close** and **Back to start**. Returning to the start runs a fresh inspection.
- Follow the Windows system theme and keep standard Qt styles for controls, dialogs, and scrolling. Use the branded banner as the product title. Keep the status and disclosure at the top and the action row at the bottom. Let the Advanced list take the available height, grow the window only as needed to fit it, and preserve the user's chosen window size when possible.
- Show the current operation, progress, and inline log during installation. Let the log grow vertically when the user resizes the window. Save diagnostic logs for each run. Keep Advanced hidden while installation is running or paused. For a field error, show a dialog with the reason, open Advanced, and highlight the affected field. Offer **Retry** and **Cancel** for other installation errors. Keep **Install** as the primary action after a failed attempt.
- Check disk space before expensive work, including the cache and destination. Preserve completed upscale work in the cache across pauses and retries. Report failures through the backend contract and keep partial output from being published as a completed install.

## Verification

- Use the [installer README](README.md) and the [Windows workflow](../.github/workflows/windows.yml) for build commands. Verify the packaged EXE by extracting it, comparing the embedded backend with the build input, and opening the window with the bundled Qt runtime.
- Verify layout changes on Windows with a test backend. Cover collapsed and expanded states, vertical and horizontal resize, display scaling, and error recovery as applicable. Use a disposable game directory for installation checks. Launch the game only with separate user authorization.
- Keep runner paths, credentials, game files, and generated captures out of the public checkout. Check observable behavior and avoid tests tied to private widget names or pixel snapshots. Commit only on request.
