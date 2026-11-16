// Aspyr 185949. Compile as freestanding x86_64-linux-gnu and extract .text.
typedef unsigned char Byte;
typedef struct { void **methods; } Object;
typedef struct { int x, y, w, h; } DisplayBounds;
typedef struct {
    int getHint, initTextures, shutdownTextures, releaseContext, createContext,
        primaryContext, mainWindow, setContextWindow, cocoaMakeCurrent,
        getCGL, setBacking, enableBacking, scheduleUpdate, updateDrawable, finish, disable,
        renderWidth, viewport, windowFlags, setFullscreen, setWindowSize, centerWindow, releaseWindow,
        setResizable, windowDisplayIndex, displayUsableBounds, windowBorders;
    char switching[32];
} Context;

enum { ShutdownTextures, InitTextures, ReleaseContext, CreateContext, ReleaseWindow, ResizeWindow };
#define GAME(field, type) ((type)((const char *)ctx + ctx->field))

static void replaceReference(Object **field, Object *next);
static void *resizeDrawable(const Context *ctx, Byte *isFullscreenContext);
static long resizeWindow(const Context *ctx, int windowed);

static void fitWindowToDisplay(const Context *ctx, void *window, int *width, int *height) {
    int display = GAME(windowDisplayIndex, int (*)(void *))(window);
    DisplayBounds usable;
    if (display < 0 || GAME(displayUsableBounds, int (*)(int, DisplayBounds *))(display, &usable))
        return;
    int top = 0, left = 0, bottom = 0, right = 0;
    if (!GAME(windowBorders, int (*)(void *, int *, int *, int *, int *))(window, &top, &left, &bottom, &right)) {
        usable.w -= left + right;
        usable.h -= top + bottom;
    }
    if (usable.w <= 0 || usable.h <= 0 || (*width <= usable.w && *height <= usable.h))
        return;
    if ((long)*width * usable.h > (long)*height * usable.w) {
        *height = (long)*height * usable.w / *width;
        *width = usable.w;
    } else {
        *width = (long)*width * usable.h / *height;
        *height = usable.h;
    }
}

void *liveDisplayRender(const Context *ctx, unsigned op, void *first, void *second,
                        void *third, void *fourth) {
    const char *hint = GAME(getHint, const char *(*)(const char *))(ctx->switching);
    int switching = hint && hint[0] == '1' && hint[1] == 0;
    switch (op) {
    case ShutdownTextures:
        if (!switching)
            GAME(shutdownTextures, void (*)(void *))(first);
        break;
    case InitTextures:
        if (!switching) {
            GAME(initTextures, void (*)(void *, void *, void *, void *))(first, second, third, fourth);
        } else {
            // Keep the texture indices and their GL resources. Only the
            // manager's owned DirectDraw and rendering-device refs change.
            Object **manager = first;
            replaceReference(&manager[2], second);
            replaceReference(&manager[3], third);
        }
        break;
    case ReleaseContext:
        if (!switching)
            GAME(releaseContext, void (*)(void *))(first);
        break;
    case CreateContext:
        if (switching)
            return resizeDrawable(ctx, second);
        return GAME(createContext, void *(*)(int, void *))((int)(long)first, second);
    case ReleaseWindow:
        if (!switching)
            GAME(releaseWindow, void (*)(void))();
        break;
    case ResizeWindow:
        return (void *)(switching ? resizeWindow(ctx, (int)(long)first) : -2L);
    }
    return 0;
}

static long resizeWindow(const Context *ctx, int windowed) {
    enum { Fullscreen = 1, FullscreenDesktop = 0x1001 };
    void *window = *GAME(mainWindow, void **);
    if (!window)
        return -1;
    int fullscreen = (GAME(windowFlags, unsigned (*)(void *))(window) & Fullscreen) != 0;
    if (fullscreen == windowed &&
        GAME(setFullscreen, int (*)(void *, unsigned))(window, windowed ? 0 : FullscreenDesktop))
        return -1;
    if (windowed) {
        const int *size = GAME(renderWidth, const int *);
        int width = size[0], height = size[1];
        // Direct3D queries the client bounds before the drawable is rebound.
        GAME(setResizable, void (*)(void *, int))(window, 1);
        fitWindowToDisplay(ctx, window, &width, &height);
        GAME(setWindowSize, void (*)(void *, int, int))(window, width, height);
        GAME(centerWindow, void (*)(void *))(window);
    }
    return 0;
}

static void replaceReference(Object **field, Object *next) {
    Object *old = *field;
    if (old == next)
        return;
    if (next)
        ((void (*)(Object *))next->methods[1])(next);
    *field = next;
    if (old)
        ((void (*)(Object *))old->methods[2])(old);
}

static void *resizeDrawable(const Context *ctx, Byte *isFullscreenContext) {
    enum { SurfaceBackingSize = 304, EnableSurfaceBackingSize = 305, ScissorTest = 0x0c11 };
    void *primary = *GAME(primaryContext, void **);
    void *window = *GAME(mainWindow, void **);
    if (!primary || !window)
        return 0;
    void *gl = *(void **)primary;
    void *cgl = GAME(getCGL, void *(*)(void *))(gl);
    if (!cgl)
        return 0;
    // Finish pending work, then discard the old drawable and reattach the
    // same context. SDL_GL_MakeCurrent would skip a repeated window/context pair.
    GAME(finish, void (*)(void))();
    GAME(setContextWindow, void (*)(void *, void *, void *))(gl, 0, 0);
    if (GAME(cocoaMakeCurrent, int (*)(void *, void *, void *))(0, window, gl))
        return 0;
    const int *size = GAME(renderWidth, const int *);
    if (GAME(setBacking, int (*)(void *, int, const int *))(cgl, SurfaceBackingSize, size) ||
        GAME(enableBacking, int (*)(void *, int))(cgl, EnableSurfaceBackingSize))
        return 0;
    GAME(scheduleUpdate, void (*)(void *))(gl);
    GAME(updateDrawable, void (*)(void *))(gl);
    // Aspyr's 2D blit uses GL_VIEWPORT height to turn top-left Y into GL Y.
    GAME(viewport, void (*)(int, int, int, int))(0, 0, size[0], size[1]);
    GAME(disable, void (*)(unsigned))(ScissorTest);
    *isFullscreenContext = 0;
    return primary;
}
