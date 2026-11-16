# Reusable Qt setup window

Build this project with CMake 3.21 or later, a C++23 compiler and Qt 6.5 or
later with Widgets. Pass the backend executable and the JSON product manifest
to `Setup-UI` in that order. The backend can be implemented in any language that can
communicate over standard streams. See [the SimCity manifest](../setup.json)
for an example. The manifest and backend should come from a trusted package.

The manifest needs a `product` display name. Optional `paths` describe
directory inputs with an `id`, a `label`, and a `browse` title.
Set `inspectOnChange` on the product directory to inspect a new selection.
A `default` may start with an environment variable placeholder such as
`${LOCALAPPDATA}`. Packaged setup supplies `${SETUP_EXE_DIR}` as the
directory containing the setup EXE or app bundle. Mark a path `required` to prompt for a
value if it is empty when the user starts an installation. Use `options` for boolean
switches, each with an `id`, `label`, and boolean `default`. An optional
`group` starts a titled group. An optional `requires` names a preceding
checkbox. Dependent options are disabled when their parent is off.
To share a manifest across systems, use `platforms` overrides keyed by
`windows` or `macos` at the top level, on `displayResolution`, or on
individual paths and options. Use `platform` on a path or option to show
it only on that system. An option can specify `backendId` to use a different
name in backend arguments and installed settings while keeping its `id`
for dependencies.
For a dropdown, provide a `values` array of string `value` and `label`
pairs and a string `default`. Optionally map system UI language codes to
dropdown values in `localeDefaults`. If `localeDefaultFiles` lists files,
the localized default requires those files beside the manifest or the setup
executable. Set `artwork` to an optional image path relative to the
manifest to show a branded header. The optional `displayResolution`
object adds a display mode picker with a flag `id`, an optional `label`,
and a `fallback` in `WIDTHxHEIGHT` format. Use `auto` to offer an Automatic
choice when the backend accepts that value. The optional `icon` path sets
the window icon. Set `completionUrl` and
`completionAction` to offer a button that opens a URL after successful
installation or configuration.
The optional `sessionLogDirectory` is an absolute path for a separate log
file per run and can contain environment variables in `${NAME}` form.
An optional HTTPS `projectUrl` adds a footer link with the label `projectLinkText`.
With `checkForUpdates` enabled and an HTTPS `projectUrl` set, setup calls
the backend's `check-update` command in the background. The command prints
a newer release tag on stdout, or nothing if no update is available. Setup
offers to open that tag on the project's releases page.

The window invokes the backend with `inspect` at startup and `install` on
request. Set `backendArgs` to prefix those commands (for example,
`["setup"]` to select a subcommand). Nonempty path inputs become `-ID PATH`
arguments. Set `reconfigure` when the backend supports `configure` and
`remove`. Both `install` and `configure` also receive the display resolution
as `-ID WIDTHxHEIGHT`, enabled dropdowns as `-ID=VALUE`, and all checkbox
options as `-ID=true` or `-ID=false`. A disabled checkbox sends false.
Set `textureArchives` to offer texture import before installation and export
after successful installation or configuration. Both actions open a file
picker and then a separate progress dialog. The backend receives
`import-textures` or `export-textures` with `-cache PATH` when provided and
`-archive PATH`.
Send one UTF-8 JSON object per line on stdout and diagnostics to stderr.

| Event `type` | Payload and behavior |
| --- | --- |
| `field` | `field` is a path ID and `path` is the value to display. An empty value clears the field. |
| `step` | `text` describes the current work. The progress bar becomes indeterminate. |
| `progress` | `value` is measured progress for the current step, from 0 to 100. The next step restores the busy indicator. |
| `count` | `value` counts units processed in the current step. An optional positive `total` sets the progress bar and displays both counts. |
| `overall` | `value` is the number of completed stages out of `total` for the operation. This updates a separate progress bar. |
| `installed` | An optional `path` is the installed launcher. To offer Configure and Remove, enable `reconfigure` and send a nonempty `settings` object. Its `options` and `choices` objects supply checkbox and dropdown values by `backendId` if set, or by `id`. Its `resolution` supplies the display resolution. |
| `removed` | Removal succeeded. Returning to the start runs a fresh inspection. |
| `textures-transferred` | The archive operation succeeded. |
| `retry-resource` | `text` describes a recoverable download failure. A line containing `retry` on stdin retries the resource in the current operation. EOF requests a pause. |
| `error` | `text` describes the problem. An optional `field` identifies a path the user can correct. |
| `stopped` | Installation paused and may be retried. |

During install, configure or an archive operation, EOF on stdin requests a pause, including when the setup
window closes. Save resumable work before publishing the final installation.
On a clean pause, emit `stopped` and exit. Successful `install` and
`configure` commands emit `installed` and exit zero. Successful `remove`,
`import-textures` and `export-textures` commands emit their corresponding
completion event and exit zero. Failed commands emit `error` and exit
nonzero. `inspect` can emit `installed` if the product is already installed.
