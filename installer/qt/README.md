# Reusable Qt setup window

Build this CMake project with a C++23 compiler and Qt 6 Widgets. Pass the
backend executable and the JSON product manifest to `Setup-UI` in that
order. The backend can be implemented in Go or any language that can
communicate over standard streams. See [the SimCity manifest](../setup.json)
for an example. The manifest and backend should come from a trusted package.

The manifest needs a `product` display name. Optional `paths` describe
directory inputs with an `id`, a `label`, and a `browse` title.
Set `inspectOnChange` on the product directory to inspect a new selection.
A `default` may start with an environment variable placeholder such as
`${LOCALAPPDATA}`. Mark a path `required` to open a directory picker when
neither the backend nor the user has supplied it. Use `options` for boolean
switches, each with an `id`, `label`, and boolean `default`. An optional
`group` starts a titled group. An optional `requires` names a preceding
checkbox. Dependent options are disabled when their parent is off.
For a dropdown, provide a `values` array of string `value` and `label`
pairs and a string `default`. Optionally map system UI language codes to
dropdown values in `localeDefaults`. If `localeDefaultFiles` lists files,
the localized default requires those files beside the manifest or the setup
executable. Set `artwork` to an optional image path relative to the
manifest to show a branded header. The optional `displayResolution`
object adds a display mode picker with a flag `id` and a `fallback` in
`WIDTHxHEIGHT` format. The optional `icon` path sets the window icon. Set `completionUrl` and
`completionAction` to offer a button that opens a URL after successful
installation or configuration.
An optional HTTPS `projectUrl` adds a footer link with the label `projectLinkText`.
With `checkForUpdates` enabled, setup calls the backend's `check-update`
command in the background. It prints a newer release tag, or nothing if no
update is available. Setup offers to open that tag on the project's releases page.

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
| `installed` | An optional `path` is the installed launcher. To offer Configure and Remove, enable `reconfigure` and send a nonempty `settings` object. Its `options` and `choices` objects supply checkbox and dropdown values by manifest ID. Its `resolution` supplies the display resolution. |
| `removed` | Removal succeeded. Returning to the start runs a fresh inspection. |
| `textures-transferred` | The archive operation succeeded. |
| `retry-resource` | `text` describes a recoverable download failure. A line containing `retry` on stdin retries the resource in the current operation. EOF requests a pause. |
| `error` | `text` describes the problem. An optional `field` identifies a path the user can correct. |
| `stopped` | Installation paused and may be retried. |

During install, configure or an archive operation, EOF on stdin requests a pause, including when the setup
window closes. Save resumable work before publishing the final installation.
On a clean pause, emit `stopped` and exit. Successful installs emit
`installed` and exit zero. Failed commands emit `error` and exit nonzero.
`inspect` can emit `installed` if the product is already installed.
