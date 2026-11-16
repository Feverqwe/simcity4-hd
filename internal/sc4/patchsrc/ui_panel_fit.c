// Compile for i386-linux-gnu and x86_64-linux-gnu as freestanding code.
// Use -fno-pic -fno-jump-tables and extract .text.
typedef unsigned U32;
typedef unsigned char Byte;
typedef struct { void **methods; } Window;
typedef struct { int left, top, right, bottom; } Rect;

#if defined(__i386__)
#define CALL __attribute__((thiscall))
#else
#define CALL
#endif

#define METHOD(window, slot, type) ((type)(window)->methods[(slot) / 4])

#if !defined(__i386__)
static int toolbarRight(Window *parent) {
    Window *toolbar = METHOD(parent, 0x8c, Window *(CALL *)(Window *, U32))(parent, 0xe9889775);
    if (!toolbar || !METHOD(toolbar, 0x11c, Byte (CALL *)(Window *))(toolbar))
        return 0;
    Window *graphs = METHOD(toolbar, 0x88, Window *(CALL *)(Window *, U32))(toolbar, 0x15200002);
    if (!graphs)
        return 0;
    // The graph button is in the rightmost column with Budget and Data Views.
    const Rect *bounds = METHOD(graphs, 0xbc, const Rect *(CALL *)(Window *))(graphs);
    int x = bounds->right - bounds->left, y = 0;
    METHOD(graphs, 0xf0, void (CALL *)(Window *, int *, int *))(graphs, &x, &y);
    METHOD(parent, 0xec, void (CALL *)(Window *, int *, int *))(parent, &x, &y);
    return x + 4;
}
#endif

void CALL moveAnchoredPanel(Window *panel, int dx, int dy) {
    const U32 id = METHOD(panel, 0xfc, U32 (CALL *)(Window *))(panel);
    // Visible horizontal extents of the doubled artwork and controls, in
    // panel coordinates. The authored roots also include anchor padding.
    int left, right;
    switch (id) {
    case 0x6a64e3c0: left = -12; right = 1020; break; // City statistics.
    case 0xaa32bce6: left = 2; right = 1034; break; // Graphs.
    case 0x8a8b5b71: case 0x8a8b5b72: left = 0; right = 1032; break; // Data views.
    case 0xaa3ac001: left = 24; right = 1056; break; // Budget ledger.
    case 0xaa3ac000: left = 570; right = 1602; break; // Monthly budget.
    case 0x6a15c767: case 0xaa15ef06: left = 572; right = 1604; break; // Advisors.
    case 0x2a1d96b1: left = 26; right = 1058; break; // Advisor messages, expanded.
    default:
        METHOD(panel, 0xe4, void (CALL *)(Window *, int, int))(panel, dx, dy);
        return;
    }
    Window *parent = METHOD(panel, 0x2c, Window *(CALL *)(Window *))(panel);
    if (parent) {
#if defined(__i386__)
        // Windows copies the bounds into the caller's rectangle.
        Rect bounds;
        METHOD(panel, 0xbc, void (CALL *)(Window *, Rect *))(panel, &bounds);
        const Rect *area = &bounds;
#else
        const Rect *area = METHOD(panel, 0xbc, const Rect *(CALL *)(Window *))(panel);
#endif
        const int width = METHOD(parent, 0xa4, int (CALL *)(Window *))(parent);
        const int limit = width - area->left - right;
        if (width >= right - left) {
#if defined(__i386__)
            // The native anchor already locates the panel during city HUD
            // creation, when child windows cannot safely be enumerated.
            const int min = -area->left - left;
#else
            const int min = toolbarRight(parent) - area->left - left;
#endif
            if (dx < min)
                dx = min;
            if (dx > limit)
                dx = limit;
        }
    }
    METHOD(panel, 0xe4, void (CALL *)(Window *, int, int))(panel, dx, dy);
}
