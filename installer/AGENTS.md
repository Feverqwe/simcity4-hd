# Installer agent guide

## Boundaries

- Keep the Qt Widgets frontend reusable. Define product options in the manifest and use the [process contract](qt/README.md) for backend communication. Steam discovery, texture processing and game modifications belong in the SimCity backend.
- Keep setup portable on both platforms. Bundle the verified backend and Qt runtime. Use the existing Steam game and profile, and offer Play through Steam. On Windows, package one EXE without installer registration, services or shortcuts.

## User flow

- Make **Install** the default path on a fresh game. Detect the game and prompt when a required value is missing or invalid. Put overrides under **Advanced options** and keep UI and World textures independently selectable.
- Refresh the detected installation when the selected game changes or the user returns to the start. Load installed values for **Configure**. Preserve the texture cache when configuring or removing the mod.
- Use platform-native Qt styling. Show the current step and inline diagnostics during work. For a field error, show the reason and expose the field. Keep retries and pauses resumable.
- Check free space before expensive work, preserve completed cache work and publish game changes only after preparation succeeds.

## Verification

- Follow the [build guide](README.md) and open the packaged setup on the target OS to verify its bundled Qt runtime. Check layout changes with a test backend at relevant display scales. Exercise collapsed and expanded states, resizing and error recovery as applicable.
- Use a disposable game directory for install checks. Launch the interactive game only with separate user authorization. Keep local paths, credentials, game files and captures out of the public checkout. Commit only on explicit request.
