# Installer agent guide

## Boundaries

- Keep the Qt Widgets frontend reusable. Define product options in the manifest and use the [process contract](qt/README.md) for backend communication. Steam discovery, texture processing and game modifications belong in the SimCity backend.
- Ship one portable Windows EXE containing the verified backend and Qt runtime. Use the existing Steam installation and game profile, and offer Play through Steam. Keep setup portable without Windows installer registration, services or installer shortcuts.

## User flow

- Make **Install** the default path on a fresh game. Detect the game and prompt when a required value is missing or invalid. Put overrides under **Advanced options** and keep UI and World textures independently selectable.
- Refresh the detected installation when the selected game changes or the user returns to the start. Load installed values for **Configure**. Preserve the texture cache when configuring or removing the mod.
- Keep setup, progress and completion distinct. Use standard Windows themed Qt controls. Show the current step and inline diagnostics during work. For a field error, show the reason and expose the field. Keep retries and pauses resumable.
- Check free space before expensive work, preserve completed cache work and publish game changes only after preparation succeeds.

## Verification

- Follow the [installer build guide](README.md) for packaging and verification of the portable EXE. Start the packaged setup on Windows to check that the bundled Qt runtime opens the window.
- Check layout changes with a test backend at relevant Windows display scales. Exercise collapsed and expanded states, resizing and error recovery as applicable.
- Use a disposable game directory for install checks. Launch the interactive game only with separate user authorization. Keep local paths, credentials, game files and captures out of the public checkout. Commit only on explicit request.
