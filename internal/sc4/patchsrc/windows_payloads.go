package patchsrc

import _ "embed"

// Windows payloads are available to builders running on any host.

//go:embed ui_draw_x4.i386
var DensityDraw []byte

//go:embed ui_draw_x2.i386
var ScreenDensityDraw []byte

//go:embed ui_source_origin.i386
var DensitySourceOrigin []byte

//go:embed ui_frame_tile.i386
var DensityTile []byte

//go:embed ui_frame_tile_x2.i386
var ScreenDensityTile []byte

//go:embed ui_budget_tree.i386
var BudgetTreeScale []byte

//go:embed ui_budget_slider.i386
var BudgetSliderArt []byte

//go:embed ui_budget_tooltip.i386
var BudgetTooltip []byte

//go:embed ui_editor_decimal.i386
var EditorDecimal []byte

//go:embed ui_import_node_font.i386
var ImportNodeFont []byte

//go:embed ui_loading_preview.i386
var ExpandLoadingPreview []byte

//go:embed ui_playlist_wheel.i386
var ScrollPlaylist []byte

//go:embed ui_map_terrain_tile.i386
var ExpandTerrainChunk []byte

//go:embed ui_map_terrain_sample.i386
var RenderTerrainChunk []byte

//go:embed ui_disasters.i386
var PrepareDisasterBackground []byte

//go:embed ui_panel_fit.i386
var WindowsPanelFitCode []byte

//go:embed ui_graphics_resolution.i386
var GraphicsResolutionCode []byte

//go:embed ui_keyboard_help.i386
var KeyboardHelpCode []byte

//go:embed ui_region_tile.i386
var ExpandRegionTile []byte
