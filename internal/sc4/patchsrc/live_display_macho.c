// Aspyr 185949. Compile for x86_64-linux-gnu with -Os -ffreestanding
// -fno-stack-protector -fno-vectorize -fno-slp-vectorize
// -fno-jump-tables -fno-asynchronous-unwind-tables. Extract .text.
typedef unsigned U32;
typedef unsigned char Byte;
typedef unsigned long UWord;
typedef struct { void **methods; } Object;
typedef struct { U32 w, h; } Size;
typedef struct { U32 left, top, right, bottom; } Rect;
typedef struct { Size size; U32 depth; } Metrics;
typedef struct {
    U32 index, width, height, depth;
    Byte timing[16];
    Byte fullscreen;
    Byte flags[23];
} VideoMode;

enum { Windowed = 1, Fullscreen, Borderless };
enum { DisplayModeShift = 26, GraphicSystem2 = 0xc47b747c };

typedef struct {
    int mainWindow, getWindowData, setWindowData, canvas, acquireGraphics,
        driverMethod, uninitializeCanvas, restoreCanvas, setFullscreen,
        enumerateWindowed, enumerateFullscreen, metrics, getDisplay, mainDisplay,
        copyDesktopMode, pixelWidth, pixelHeight, releaseMode, videoDevice, systemLocked,
        getPreference, putPreference, syncPreferences, inWindow, windowManager,
        primaryContext, setHint, pauseDraw, appClass, regionClass, alignWindow, cameraViewport,
        updateRegionPosition, rendererClass, rendererBackingStore, rendererDriver, resetViewport,
        createBackingStore, glGetProcAddress, glBindFramebuffer, glDeleteFramebuffers,
        region3DClass, screenBufferClass, drawBufferClass, drawContextClass, simGLDrawClass,
        resetDrawClip, recomputeDrawClip, regionMapClass, scrollRegionTiles, invalidateRegionTiles,
        computeRegionLimits, backingStoreEnabled, updateCityView, updateCityScrollBounds, parseWindowID;
    char preference[16], pending[24], failed[24];
    char switching[32], enabled[2], disabled[2];
    char getAttachments[48], deleteRenderbuffers[28];
    char sceneRedraw[24];
} Context;

typedef struct {
    U32 name;
    void (*getAttachment)(U32, U32, U32, int *);
    void (*deleteBuffers)(int, const U32 *);
} BackingStore;

#define GAME(field, type) ((type)((const char *)ctx + ctx->field))
#define FIELD(object, offset, type) (*(type *)((char *)(object) + (offset)))
#define METHOD(object, offset, type) ((type)(object)->methods[(offset)/8])

static void applyDisplay(const Context *ctx, Object *canvas, U32 request);

void liveDisplay(const Context *ctx, Object *renderer) {
    void *window = GAME(mainWindow, void *(*)(void))();
    if (renderer) {
        void *pending = window ? GAME(getWindowData, void *(*)(void *, const char *))(window, ctx->sceneRedraw) : 0;
        Byte *cache = GAME(backingStoreEnabled, Byte *);
        Byte saved = *cache;
        if (pending)
            GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->sceneRedraw, 0);
        // The camera can move before the first draw at the new size.
        // Draw directly once before reusing the framebuffer cache.
        if (pending == renderer)
            *cache = 0;
        METHOD(renderer, 0xa8, Byte (*)(Object *))(renderer);
        if (pending == renderer)
            *cache = saved;
        return;
    }
    Object *canvas = *GAME(canvas, Object **);
    if (!canvas || !window)
        return;
    U32 request = (UWord)GAME(getWindowData, void *(*)(void *, const char *))(window, ctx->pending);
    if (!request || GAME(systemLocked, Byte (*)(void))())
        return;
    GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->pending, 0);
    applyDisplay(ctx, canvas, request);
}

static Size desktopSize(const Context *ctx) {
    U32 display = GAME(getDisplay, U32 (*)(void))();
    if (!display)
        display = GAME(mainDisplay, U32 (*)(void))();
    const void *mode = GAME(copyDesktopMode, const void *(*)(U32))(display);
    if (!mode)
        return (Size){0, 0};
    UWord width = GAME(pixelWidth, UWord (*)(const void *))(mode);
    UWord height = GAME(pixelHeight, UWord (*)(const void *))(mode);
    GAME(releaseMode, void (*)(const void *))(mode);
    return (Size){(U32)width, (U32)height};
}

static int ensureMode(const Context *ctx, Object *driverInterface, Metrics metrics, Byte fullscreen) {
    // Only the DX7 driver adapts the mode list to Aspyr's OpenGL layer.
    if (!driverInterface || driverInterface->methods[0x210 / 8] != GAME(driverMethod, void *))
        return 0;
    void *driver = (char *)driverInterface - 0x18;
    const VideoMode *end = FIELD(driver, 0x140, const VideoMode *);
    for (const VideoMode *mode = FIELD(driver, 0x138, const VideoMode *); mode != end; ++mode) {
        if (mode->width == metrics.size.w && mode->height == metrics.size.h &&
            mode->depth == metrics.depth && mode->fullscreen == fullscreen)
            return 1;
    }
    // The driver callbacks read only width, height and depth from DDSURFACEDESC2.
    U32 descriptor[24];
    descriptor[2] = metrics.size.h;
    descriptor[3] = metrics.size.w;
    descriptor[0x5c / 4] = metrics.depth;
    typedef int (*Enumerate)(U32 *, void *);
    Enumerate enumerate = fullscreen ? GAME(enumerateFullscreen, Enumerate) : GAME(enumerateWindowed, Enumerate);
    return enumerate(descriptor, driver) != 0;
}

static void fitWindow(Object *window, Size size) {
    if (window)
        METHOD(window, 0x1b0, void (*)(Object *, int, int, int, int))(window, 0, 0, size.w, size.h);
}

static Object *childWindow(Object *window, U32 id) {
    return METHOD(window, 0x110, Object *(*)(Object *, U32))(window, id);
}

static Byte alignChild(Object *parent, U32 id, Object *child, const Context *ctx) {
    if (id && METHOD(child, 0x118, Object *(*)(Object *, U32))(child, 0xaaaa))
        GAME(alignWindow, Byte (*)(U32, U32, U32, Object *))(id, 0xaaaa, 0, parent);
    return 1;
}

static Byte alignCityWindow(Object *screen, U32 id, Object *window, const Context *ctx, U32 remaining) {
    if (!remaining)
        return 0; // A cycle cannot be longer than the number of top-level windows.
    Object *marker = METHOD(window, 0x118, Object *(*)(Object *, U32))(window, 0xaaaa);
    if (!marker || !id)
        return 1;
    Object *caption = METHOD(marker, 0x248, Object *(*)(Object *))(marker);
    const char *text = METHOD(caption, 0x38, const char *(*)(Object *))(caption);
    if (text && text[0] != '-') {
        U32 target = GAME(parseWindowID, U32 (*)(const char *))(text);
        Object *anchor = target ? METHOD(screen, 0x118, Object *(*)(Object *, U32))(screen, target) : 0;
        // Align the window containing the target before using its position.
        while (anchor && anchor != screen) {
            Object *parent = METHOD(anchor, 0x58, Object *(*)(Object *))(anchor);
            if (parent == screen) {
                U32 anchorID = METHOD(anchor, 0x1f8, U32 (*)(Object *))(anchor);
                if (!alignCityWindow(screen, anchorID, anchor, ctx, remaining - 1))
                    return 0;
                break;
            }
            anchor = parent;
        }
    }
    GAME(alignWindow, Byte (*)(U32, U32, U32, Object *))(id, 0xaaaa, 0, screen);
    return 1;
}

static Byte alignCityChild(Object *screen, U32 id, Object *child, const Context *ctx) {
    U32 count = METHOD(screen, 0x68, U32 (*)(Object *))(screen);
    alignCityWindow(screen, id, child, ctx, count);
    return 1;
}

typedef Byte (*ChildCallback)(Object *, U32, Object *, const Context *);

static void alignChildren(const Context *ctx, Object *window, ChildCallback align) {
    METHOD(window, 0x100, Byte (*)(Object *, U32, ChildCallback, const Context *))(
        window, 0x22ba0121, align, ctx); // EnumChildren(IGZWin).
}

static void positionPanel(Object *panel, int x, int y, int right) {
    if (!panel)
        return;
    if (right)
        x -= METHOD(panel, 0x148, int (*)(Object *))(panel);
    y -= METHOD(panel, 0x150, int (*)(Object *))(panel);
    METHOD(panel, 0x1c0, void (*)(Object *, int, int))(panel, x, y);
}

static void resizeRenderer(const Context *ctx, Object *renderer, Size size) {
    if (!renderer || renderer->methods != GAME(rendererClass, void **))
        return;
    METHOD(renderer, 0x60, void (*)(Object *, U32, U32))(renderer, size.w, size.h);
    // Discard a queued translated update and redraw into the new backing store.
    FIELD(renderer, 0x13c, int) = 0;
    FIELD(renderer, 0x140, int) = 0;
    FIELD(renderer, 0xb1, Byte) = 0; // Contents of the backing store are invalid.
    if (FIELD(renderer, 0x180, void *)) { // Draw context is assigned during scene initialization.
        FIELD(renderer, 0x165, Byte) = 0; // Rebuild the frustum even if a full update was queued.
        METHOD(renderer, 0xa0, void (*)(Object *))(renderer);
        void *window = GAME(mainWindow, void *(*)(void))();
        if (window)
            GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->sceneRedraw, renderer);
    }
}

static void resizeCityRenderer(const Context *ctx, void *city, Size size) {
    void *camera = FIELD(city, 0x180, void *);
    if (camera)
        GAME(cameraViewport, void (*)(void *, int, int))(camera, size.w, size.h);
    Object *renderer = FIELD(city, 0x188, Object *);
    if (!renderer || renderer->methods != GAME(rendererClass, void **))
        return;
    void *view = FIELD(renderer, 0x170, void *);
    if (camera && FIELD(camera, 0x10, void *) && view) {
        GAME(updateCityView, void (*)(void *, void *))(view, camera);
        GAME(updateCityScrollBounds, void (*)(void *))(camera);
    }
    // After the drawable is replaced, the translated update can leave
    // holes in the newly exposed strips. Keep full-frame cache transfers.
    FIELD(renderer, 0xb0, Byte) = 0;
    resizeRenderer(ctx, renderer, size);
}

static void resizeScreenBuffer(const Context *ctx, Object *graphics, Size size) {
    Object *screen = FIELD(graphics, 0x168, Object *);
    if (!screen || screen->methods != GAME(screenBufferClass, void **))
        return;
    Rect bounds = {0, 0, size.w, size.h};
    // The driver has replaced the DirectDraw surfaces. Refresh the three
    // rectangles that cGZScreenBuffer::Init normally sets at startup.
    FIELD(screen, 0x1c, Rect) = bounds;
    FIELD(screen, 0x6c, Rect) = bounds;
    FIELD(screen, 0x7c, Rect) = bounds;

    Object *buffer = METHOD(screen, 0x200, Object *(*)(Object *))(screen); // ActiveDrawBuffer.
    if (!buffer || buffer->methods != GAME(drawBufferClass, void **) ||
        FIELD(buffer, 0x28, Object *) != screen)
        return;
    FIELD(buffer, 0x30, Rect) = bounds;
    FIELD(buffer, 0x58, Size) = size;
    Object *draw = FIELD(buffer, 0x20, Object *);
    if (!draw || draw->methods != GAME(drawContextClass, void **))
        return;
    Object *renderer = FIELD(draw, 0x30, Object *);
    if (renderer && renderer->methods == GAME(simGLDrawClass, void **))
        FIELD(renderer, 0x18, Size) = size;
    FIELD(draw, 0x60, Rect) = bounds; // ClipReset copies this canvas into the active clip.
    GAME(resetDrawClip, void (*)(Object *))(draw);
    if (renderer)
        GAME(recomputeDrawClip, void (*)(Object *))(draw);
}

static Object *gameApp(const Context *ctx, Object *root) {
    Object *app = METHOD(root, 0x118, Object *(*)(Object *, U32))(root, 0x6104489a);
    return app && app->methods == GAME(appClass, void **) ? app : 0;
}

static void layoutGameScreen(const Context *ctx, Object *root, Size size) {
    Object *app = gameApp(ctx, root);
    if (!app)
        return;
    fitWindow(app, size);
    Object *screen = FIELD(app, 0x178, Object *);
    if (!screen)
        return;
    fitWindow(screen, size);
    if (screen->methods == GAME(regionClass, void **)) {
        // RegionScreen::Init sizes these layers and computes pan limits once.
        Object *map = FIELD(screen, 0x160, Object *);
        fitWindow(map, size);
        fitWindow(FIELD(screen, 0x168, Object *), size);
        Object *backdrop = FIELD(screen, 0x170, Object *);
        fitWindow(backdrop, size);
        fitWindow(FIELD(screen, 0x288, Object *), size);
        void *camera = FIELD(screen, 0x268, void *);
        if (camera) {
            GAME(cameraViewport, void (*)(void *, int, int))(camera, size.w, size.h);
            if (FIELD(screen, 0x270, void *)) {
                GAME(computeRegionLimits, void (*)(Object *))(screen);
                GAME(updateRegionPosition, void (*)(Object *))(screen);
            }
        }
        if (backdrop && backdrop->methods == GAME(region3DClass, void **))
            resizeRenderer(ctx, FIELD(backdrop, 0x150, Object *), size);
        if (map && map->methods == GAME(regionMapClass, void **)) {
            Object *tiles = FIELD(map, 0x1a0, Object *);
            if (tiles) {
                // SetArea resets the tile origin and leaves the map's scroll
                // position intact. Resync the canvas before drawing new tiles.
                GAME(scrollRegionTiles, void (*)(Object *, int, int))(
                    tiles, FIELD(map, 0x170, int), FIELD(map, 0x174, int));
                GAME(invalidateRegionTiles, void (*)(Object *))(map);
            }
        }
        // LoadGUI uses explicit positions for the summary and tutorial panels.
        positionPanel(childWindow(screen, 0x09ebe9ee), 5 + (size.w > 3 * size.h ? size.w / 3 : 0), size.h + 2, 0);
        positionPanel(childWindow(screen, 0x0bb0f5e7), size.w - 5, size.h - 5, 1);
        positionPanel(childWindow(screen, 0x6bb92bca), size.w - 10, size.h - 10, 1);
    } else {
        void *city = FIELD(app, 0x180, void *);
        if (city && (char *)city + 0x18 == (char *)screen) {
            resizeCityRenderer(ctx, city, size);
            // AddGUI positions Query Help and its caption explicitly, and sizes
            // the full-screen effect window independently of the city view.
            positionPanel(FIELD(city, 0x280, Object *),
                10 + (size.w > 3 * size.h ? size.w / 3 : 0), size.h + 22, 0);
            Object *caption = childWindow(screen, 0x43);
            if (caption)
                METHOD(caption, 0x1c0, void (*)(Object *, int, int))(caption, 24, size.h - 56);
            fitWindow(childWindow(screen, 0x6a5e44b6), size);
            alignChildren(ctx, screen, alignCityChild);
            return;
        }
    }
    alignChildren(ctx, screen, alignChild);
}

static int reopenDisplay(const Context *ctx, Object *canvas, Object *graphics, void *driver, Rect bounds,
                         Metrics metrics, U32 mode, int *spacesAllowed, int spaces) {
    void *window = GAME(mainWindow, void *(*)(void))();
    if (!window)
        return 0;
    // Leave an existing fullscreen Space using its original policy before
    // SDL enters the other fullscreen mode on this same window.
    if (!*GAME(inWindow, Byte *) && *spacesAllowed != spaces &&
        GAME(setFullscreen, int (*)(void *, U32))(window, 0))
        return 0;
    GAME(uninitializeCanvas, Byte (*)(Object *))(canvas);
    *spacesAllowed = spaces;
    FIELD(graphics, 0x80, Rect) = bounds;
    FIELD(graphics, 0x10a, Byte) = mode != Windowed; // cGZGraphicSystem::IsFullScreenMode.
    *GAME(metrics, Metrics *) = metrics;
    return GAME(restoreCanvas, Byte (*)(Object *))(canvas) && FIELD(driver, 0x288, void *);
}

static void refreshGameScreen(const Context *ctx, Object *graphics, Size size) {
    resizeScreenBuffer(ctx, graphics, size);
    // Restore resizes the root. Fit the game screens to it before rebinding
    // the window tree to the new buffers and queuing a complete paint.
    Object *windows = GAME(windowManager, Object *(*)(void))();
    if (windows) {
        Object *root = METHOD(windows, 0x18, Object *(*)(Object *))(windows); // GetMainWindow.
        if (root) {
            layoutGameScreen(ctx, root, size);
            METHOD(root, 0x300, void (*)(Object *))(root); // SetBufferToDrawToRecursive.
        }
        METHOD(windows, 0x1e0, void (*)(Object *))(windows); // InvalidateAllWindows.
    }
}

static void deleteBackingStore(const Context *ctx, const BackingStore *store, U32 name) {
    if (!name)
        return;
    enum { Framebuffer = 0x8d40, Color = 0x8ce0, Depth = 0x8d00, ObjectName = 0x8cd1 };
    void (*bind)(U32, U32) = GAME(glBindFramebuffer, void (*)(U32, U32));
    int color = 0, depth = 0;
    bind(Framebuffer, name);
    store->getAttachment(Framebuffer, Color, ObjectName, &color);
    store->getAttachment(Framebuffer, Depth, ObjectName, &depth);
    bind(Framebuffer, 0);
    GAME(glDeleteFramebuffers, void (*)(int, const U32 *))(1, &name);
    U32 buffers[] = {(U32)color, (U32)depth};
    store->deleteBuffers(2, buffers);
}

static int prepareBackingStore(const Context *ctx, Object *driver, Size size, BackingStore *next) {
    U32 *current = GAME(rendererBackingStore, U32 *);
    if (!*current)
        return 1;
    if (*GAME(rendererDriver, Object **) != driver)
        return 0;
    GAME(resetViewport, void (*)(Object *))(driver);
    Rect viewport = METHOD(driver, 0x260, Rect (*)(Object *))(driver);
    if (viewport.right - viewport.left != size.w || viewport.bottom - viewport.top != size.h)
        return 0;
    void *(*getProc)(const char *) = GAME(glGetProcAddress, void *(*)(const char *));
    next->getAttachment = getProc(ctx->getAttachments);
    next->deleteBuffers = getProc(ctx->deleteRenderbuffers);
    if (!next->getAttachment || !next->deleteBuffers)
        return 0;

    // Aspyr creates a color/depth pair sized to the driver's viewport and
    // publishes the FBO through this global. Hold both until the mode commits.
    U32 previous = *current;
    *current = 0;
    Byte complete = GAME(createBackingStore, Byte (*)(void))();
    next->name = *current;
    *current = previous;
    return complete && next->name;
}

static int saveDisplay(const Context *ctx, U32 request) {
    U32 saved = GAME(getPreference, int (*)(const char *, int))(ctx->preference, 0);
    if (saved == request)
        return 1;
    GAME(putPreference, void (*)(const char *, int))(ctx->preference, request);
    if (GAME(syncPreferences, Byte (*)(void))())
        return 1;
    GAME(putPreference, void (*)(const char *, int))(ctx->preference, saved);
    return 0;
}

static int switchDisplay(const Context *ctx, Object *canvas, U32 request) {
    U32 mode = request >> DisplayModeShift;
    if (mode < Windowed || mode > Borderless)
        return 0;
    Size size = mode == Borderless ? desktopSize(ctx) : (Size){request & 8191, (request >> 13) & 8191};
    if (size.w < 800 || size.w > 7680 || size.h < 600 || size.h > 4320)
        return 0;

    // The bundled SDL reads the Spaces hint only during Cocoa_VideoInit.
    void *device = GAME(videoDevice, void *(*)(void))();
    const char *backend = device ? FIELD(device, 0, const char *) : 0;
    if (!backend || backend[0] != 'c' || backend[1] != 'o' || backend[2] != 'c' ||
        backend[3] != 'o' || backend[4] != 'a' || backend[5] != 0)
        return 0;
    int *spacesAllowed = FIELD(device, 0x530, int *);
    if (!spacesAllowed)
        return 0;
    Object *interface = GAME(acquireGraphics, Object *(*)(void))();
    Object *graphics2 = 0;
    if (!interface || !METHOD(interface, 0, Byte (*)(Object *, U32, Object **))(
            interface, GraphicSystem2, &graphics2))
        return 0;
    Object *driverInterface = METHOD(graphics2, 0x18, Object *(*)(Object *))(graphics2);
    Object *graphics = (Object *)((char *)graphics2 - 0x30);
    Rect before = FIELD(graphics, 0x80, Rect);
    int previousSpaces = *spacesAllowed;
    U32 previousMode = *GAME(inWindow, Byte *) ? Windowed : (previousSpaces ? Fullscreen : Borderless);
    Metrics previousMetrics = *GAME(metrics, Metrics *);
    Metrics target = {size, 32};
    int same = previousMode == mode && before.right - before.left == size.w &&
        before.bottom - before.top == size.h;
    int ok = 0;
    if (same) {
        ok = saveDisplay(ctx, request);
    } else if (*GAME(primaryContext, void **) && ensureMode(ctx, driverInterface, target, mode != Windowed) &&
               GAME(setHint, Byte (*)(const char *, const char *, int))(ctx->switching, ctx->enabled, 2)) {
        void *driver = (char *)driverInterface - 0x18;
        Rect bounds = {0, 0, size.w, size.h};
        BackingStore next;
        next.name = 0;
        ok = reopenDisplay(ctx, canvas, graphics, driver, bounds, target, mode, spacesAllowed, mode != Borderless) &&
            prepareBackingStore(ctx, driverInterface, size, &next) &&
            saveDisplay(ctx, request);
        U32 discarded = next.name;
        if (ok && next.name) {
            U32 *current = GAME(rendererBackingStore, U32 *);
            discarded = *current;
            *current = next.name;
        }
        deleteBackingStore(ctx, &next, discarded);
        if (ok) {
            refreshGameScreen(ctx, graphics, size);
        } else if (reopenDisplay(ctx, canvas, graphics, driver, before, previousMetrics, previousMode,
                                 spacesAllowed, previousSpaces)) {
            refreshGameScreen(ctx, graphics, previousMetrics.size);
        } else if (!FIELD(driver, 0x288, void *)) {
            GAME(pauseDraw, void (*)(Object *))(graphics);
        }
        GAME(setHint, Byte (*)(const char *, const char *, int))(ctx->switching, ctx->disabled, 2);
    }
    METHOD(graphics2, 0x10, void (*)(Object *))(graphics2);
    return ok;
}

static void applyDisplay(const Context *ctx, Object *canvas, U32 request) {
    int ok = switchDisplay(ctx, canvas, request);
    // Applying Graphics Options can leave keyboard focus on the inert
    // desktop window after the dialog closes. Return it to the region.
    Object *windows = GAME(windowManager, Object *(*)(void))();
    if (windows) {
        Object *root = METHOD(windows, 0x18, Object *(*)(Object *))(windows);
        Object *focus = METHOD(windows, 0x120, Object *(*)(Object *))(windows); // GZGetFocus.
        if (root && (!focus || focus == root)) {
            Object *app = gameApp(ctx, root);
            Object *screen = app ? FIELD(app, 0x178, Object *) : 0;
            if (screen && screen->methods == GAME(regionClass, void **))
                METHOD(windows, 0x128, void (*)(Object *, Object *))(windows, screen); // GZSetFocus.
        }
    }
    void *window = GAME(mainWindow, void *(*)(void))();
    if (window)
        GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->failed, ok ? 0 : (void *)1);
}
