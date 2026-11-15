# Installer agent guide

## Boundaries

- Keep the installer a separate subproject. The Qt Widgets frontend calls the built Go backend through the [manifest and process contract](qt/README.md). Define product branding and options in the manifest. Keep Steam discovery, texture processing and game modifications in the SimCity backend under `../internal/sc4/`.
- Ship one portable Windows EXE containing the verified backend and Qt runtime. Prepare the banner at packaging time and keep the source image available for the public README.
- Apply the default HD setup to the existing Steam installation and keep the existing game profile. Write editable settings where the relevant DLL reads them. Launch the game through Steam. Keep setup portable by avoiding Windows installer registration, persistent services and installer shortcuts.

## User flow

- Make **Install** the default path. Detect the game, choose defaults and prompt only when a required value is missing or invalid. Put directory overrides, resolution and grouped controls under **Advanced options**. Keep UI upscaling and world textures independently selectable.
- Inspect the game folder on startup and when the selection changes. Load installed settings before offering **Configure** or **Remove mod**. Reuse assembled packages where applicable and preserve the texture cache across both operations.
- Keep setup, progress and completion distinct. After success, offer **Close** and **Back to start**, with **Play** after installation or configuration. Refresh the inspection when returning to the start.
- Use the Windows theme and standard Qt controls. Use the banner as the title, place status and disclosure above the content and keep actions at the bottom. Let Advanced and the progress log use available height and preserve the user's window size where possible.
- Show the current step, progress and inline log. Save diagnostics per run and hide Advanced while an operation is active or paused. For a field error, show the reason, open Advanced and highlight the affected field. For retryable operation failures, offer **Retry** and **Cancel**. Keep **Install** available after a failed initial install.
- Check free space for the cache and destination before expensive work. Preserve completed cache work on pause and retry. Report failures through the backend contract and publish the installation only after it completes.

## Verification

- Follow the [installer build guide](README.md) for packaging. The [Windows workflow](../.github/workflows/windows.yml) verifies the bundled backend and translation after extraction. Start the packaged setup on Windows to check that the bundled Qt runtime opens the window.
- Check layout changes with a test backend at relevant Windows display scales. Exercise collapsed and expanded states, resizing in both directions and error recovery as applicable. Use a disposable game directory for installation checks. Launch the game only with separate user authorization.
- Keep runner paths, credentials, game files and generated captures outside the public checkout. Test observable behavior. Commit only on explicit request.
