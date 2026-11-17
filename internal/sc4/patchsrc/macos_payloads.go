package patchsrc

import _ "embed"

// macOS payloads are available to builders running on any host.

//go:embed ui_density_macho.x86_64
var MacDensityCode []byte

//go:embed launch_macho.x86_64
var MacLaunchCode []byte

//go:embed display_mode_macho.x86_64
var MacDisplayModeCode []byte

//go:embed keyboard_help_macho.x86_64
var MacKeyboardHelpCode []byte

//go:embed ui_runtime_art_macho.x86_64
var MacRuntimeArtworkCode []byte

//go:embed ui_disasters.x86_64
var MacDisasterBackgroundCode []byte

//go:embed ui_playlist_wheel.x86_64
var MacPlaylistWheelCode []byte

//go:embed editor_decimal_macho.x86_64
var MacEditorDecimalCode []byte

//go:embed ui_budget_tooltip_macho.x86_64
var MacBudgetTooltipCode []byte

//go:embed ui_budget_tree_macho.x86_64
var MacBudgetTreeCode []byte

//go:embed ui_combo_arrow_macho.x86_64
var MacComboCode []byte

//go:embed ui_cursors_macho.x86_64
var MacCursorCode []byte

//go:embed ui_map_terrain_macho.x86_64
var MacDataViewTerrainCode []byte

//go:embed data_view_legend_macho.x86_64
var MacDataViewLegendCode []byte

//go:embed data_view_coverage_macho.x86_64
var MacDataViewCoverageCode []byte

//go:embed display_options_macho.x86_64
var MacDisplayOptionsCode []byte

//go:embed ui_font_metrics_macho.x86_64
var MacFontMetricsCode []byte

//go:embed ui_graphs_macho.x86_64
var MacGraphsCode []byte

//go:embed ui_html_macho.x86_64
var MacHTMLScrollbarsCode []byte

//go:embed ui_import_node_macho.x86_64
var MacImportNodeCode []byte

//go:embed live_display_macho.x86_64
var MacLiveDisplayCode []byte

//go:embed live_display_render_macho.x86_64
var MacLiveDisplayRenderCode []byte

//go:embed ui_projection_macho.x86_64
var MacUIProjectionCode []byte

//go:embed ui_loading_preview.x86_64
var MacLoadingPreviewCode []byte

//go:embed ui_region_tile.x86_64
var MacRegionTileCode []byte

//go:embed window_resize_macho.x86_64
var MacWindowResizeCode []byte

//go:embed ui_font_atlas_macho.x86_64
var MacRetinaAtlas []byte

//go:embed retina_frame_macho.x86_64
var MacRetinaFrame []byte

//go:embed ui_snapshot_macho.x86_64
var MacSnapshotCode []byte

//go:embed ui_panel_fit.x86_64
var MacPanelFitCode []byte

//go:embed ui_catalog_paint.x86_64
var MacCatalogPaint []byte

//go:embed ui_catalog_tile.x86_64
var MacCatalogTile []byte

//go:embed ui_catalog_height.x86_64
var MacCatalogHeight []byte

//go:embed ui_catalog_button.x86_64
var MacCatalogButton []byte
