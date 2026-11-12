# Reusable Qt setup window

Build this CMake project with a C++23 compiler and Qt 6 Widgets. Launch `Setup-UI` with two
arguments: the path to a backend executable and the path to a JSON product
manifest. The backend can be implemented in Go or any language that can
communicate over standard streams. See [the SimCity manifest](../setup.json)
for an example. The manifest and backend should come from a trusted package.

The manifest needs a `product` display name. Optional `paths` describe
directory inputs; each has an `id`, a `label`, and a `browse` title.
Set `inspectOnChange` on the product directory to inspect a new selection.
A `default` may start with an environment variable placeholder such as
`${LOCALAPPDATA}`. Mark a path `required` to open a directory picker when
neither the backend nor the user has supplied it. Use `options` for boolean
switches, each with an `id`, `label`, and boolean `default`. An optional
`group` starts a titled group. An optional `requires` names a preceding
checkbox; dependent options are disabled when their parent is off.
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

The window invokes the backend with `inspect` at startup and `install` on
request. Set `backendArgs` to prefix those commands (for example,
`["setup"]` to select a subcommand). Nonempty path inputs become `-ID PATH` arguments. Boolean options
are sent for `install` and `configure` as `-ID=true` or `-ID=false`.
Enabled dropdowns send `-ID=VALUE`. Set
`reconfigure` in the manifest when the backend supports `configure` and
`remove`. Configure receives the same flags as install. Remove receives
the configured paths. Send one UTF-8 JSON
object per line on stdout and send diagnostics to stderr.

| Event `type` | Payload and behavior |
| --- | --- |
| `field` | `field` is a path ID; `path` is the detected or validated value. An empty value requests user input for a required path. |
| `step` | `phase` is a one-based stage number; `text` describes the current work. |
| `progress` | `value` is measured progress for the current step, from 0 to 100. The next step restores the busy indicator. |
| `count` | `value` counts units processed in the current step. An optional positive `total` sets the progress bar and displays both counts. |
| `installed` | An optional `path` is the installed launcher. To offer Configure and Remove, include `settings` with an `options` object of booleans keyed by manifest option ID. Optional `choices` supplies dropdown values by ID, and `resolution` supplies the display resolution. |
| `removed` | Removal succeeded. Returning to the start runs a fresh inspection. |
| `retry-resource` | `text` describes a recoverable download failure. Wait for a line containing `retry` on stdin to retry the resource in the current operation. |
| `error` | `text` describes the problem; optional `field` identifies a path the user can correct. |
| `stopped` | Installation paused and may be retried. |

During install or configure, EOF on stdin requests a pause, including when the setup
window closes. Save resumable work before publishing the final installation.
On a clean pause, emit `stopped` and exit. Successful installs emit
`installed` and exit zero. Failed commands emit `error` and exit nonzero.
`inspect` can emit `installed` if the product is already installed.
