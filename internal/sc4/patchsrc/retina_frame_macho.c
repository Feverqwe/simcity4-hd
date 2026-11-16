// Aspyr 185949. Logical display dimensions stay in SDL, DirectDraw and the UI.
// Only operations on the OpenGL drawable or its full-frame cache are scaled.
// Compile with -target x86_64-linux-gnu -Os -ffreestanding -fno-pic
// -fno-stack-protector -fno-asynchronous-unwind-tables -fno-vectorize -fno-slp-vectorize.
// Extract relocation-free .text with llvm-objcopy.
typedef unsigned char Byte;
typedef unsigned U32;
typedef struct {
    void *context;
    U32 capture, color, depth;
    int scale;
    void (*pixelZoom)(float, float);
    void (*deleteRenderbuffers)(int, const U32 *);
} FrameState;

typedef struct {
    int state, getProc, cglSet, cglEnable, getInteger, isEnabled, enable, disable,
        bindFrame, bindRenderbuffer, checkFrame, genFrame, genRenderbuffer,
        deleteFrame, storage, attach, blit, read, viewport, scissor, windowPosition,
        copy, draw, pushAttrib, popAttrib;
    char pixelZoom[16], deleteRenderbuffers[32];
} FrameAPI;
_Static_assert(sizeof(FrameState) == 40, "Aspyr frame state");
_Static_assert(__builtin_offsetof(FrameState, scale) == 20, "Aspyr frame scale");

#define NATIVE(field, type) ((type)((const char *)api + api->field))
#define STATE NATIVE(state, FrameState *)

enum {
    Framebuffer = 0x8d40, DrawFramebuffer = 0x8ca9, ReadFramebuffer = 0x8ca8,
    DrawBinding = 0x8ca6, ReadBinding = 0x8caa, Renderbuffer = 0x8d41,
    RenderbufferBinding = 0x8ca7, RGBA8 = 0x8058, Depth24 = 0x81a6,
    ColorAttachment = 0x8ce0, DepthAttachment = 0x8d00, Complete = 0x8cd5,
    MaxViewport = 0x0d3a, MaxRenderbuffer = 0x84e8, Viewport = 0xba2, ScissorTest = 0xc11,
    ColorBuffer = 0x4000, DepthBuffer = 0x100, DepthComponent = 0x1902,
    Linear = 0x2601, Nearest = 0x2600, PixelMode = 0x20,
    SurfaceBackingSize = 304, EnableSurfaceBackingSize = 305,
};

static int pixelScale(const FrameAPI *api) {
    return STATE->scale == 2 ? 2 : 1;
}

static void discardCapture(const FrameAPI *api, const FrameState *state) {
    if (state->capture) {
        NATIVE(deleteFrame, void (*)(int, const U32 *))(1, &state->capture);
        const U32 buffers[] = {state->color, state->depth};
        state->deleteRenderbuffers(2, buffers);
    }
}

static int createCapture(const FrameAPI *api, FrameState *state, int width, int height) {
    void (*getInteger)(U32, int *) = NATIVE(getInteger, void (*)(U32, int *));
    int oldRead, oldDraw, oldRenderbuffer;
    getInteger(ReadBinding, &oldRead);
    getInteger(DrawBinding, &oldDraw);
    getInteger(RenderbufferBinding, &oldRenderbuffer);
    void (*bind)(U32, U32) = NATIVE(bindFrame, void (*)(U32, U32));
    void (*bindRenderbuffer)(U32, U32) = NATIVE(bindRenderbuffer, void (*)(U32, U32));
    void (*generate)(int, U32 *) = NATIVE(genRenderbuffer, void (*)(int, U32 *));
    void (*storage)(U32, U32, int, int) = NATIVE(storage, void (*)(U32, U32, int, int));
    void (*attach)(U32, U32, U32, U32) = NATIVE(attach, void (*)(U32, U32, U32, U32));

    NATIVE(genFrame, void (*)(int, U32 *))(1, &state->capture);
    if (!state->capture)
        return 0;
    bind(Framebuffer, state->capture);
    generate(1, &state->color);
    bindRenderbuffer(Renderbuffer, state->color);
    storage(Renderbuffer, RGBA8, width, height);
    attach(Framebuffer, ColorAttachment, Renderbuffer, state->color);
    generate(1, &state->depth);
    bindRenderbuffer(Renderbuffer, state->depth);
    storage(Renderbuffer, Depth24, width, height);
    attach(Framebuffer, DepthAttachment, Renderbuffer, state->depth);
    int complete = state->color && state->depth && NATIVE(checkFrame, U32 (*)(U32))(Framebuffer) == Complete;
    bindRenderbuffer(Renderbuffer, oldRenderbuffer);
    bind(ReadFramebuffer, oldRead);
    bind(DrawFramebuffer, oldDraw);
    return complete;
}

int setRetinaBacking(void *context, int parameter, const int *size, const FrameAPI *api) {
    int (*set)(void *, int, const int *) = NATIVE(cglSet, int (*)(void *, int, const int *));
    int (*enable)(void *, int) = NATIVE(cglEnable, int (*)(void *, int));
    if (parameter != SurfaceBackingSize)
        return set(context, parameter, size);

    int limits[2] = {0, 0}, renderbuffer = 0;
    void (*getInteger)(U32, int *) = NATIVE(getInteger, void (*)(U32, int *));
    getInteger(MaxViewport, limits);
    getInteger(MaxRenderbuffer, &renderbuffer);
    FrameState next;
    next.context = context;
    next.capture = next.color = next.depth = 0;
    next.scale = 1;
    void *(*proc)(const char *) = NATIVE(getProc, void *(*)(const char *));
    next.pixelZoom = proc(api->pixelZoom);
    next.deleteRenderbuffers = proc(api->deleteRenderbuffers);
    int width = size[0], height = size[1];
    int ready = width > 0 && height > 0 && width <= limits[0]/2 && height <= limits[1]/2 &&
                width <= renderbuffer/2 && height <= renderbuffer/2 &&
                next.pixelZoom && next.deleteRenderbuffers;
    int result;
    if (ready && createCapture(api, &next, width, height)) {
        int physical[] = {width * 2, height * 2};
        result = set(context, parameter, physical);
        if (!result)
            result = enable(context, EnableSurfaceBackingSize);
        if (!result)
            next.scale = 2;
    }
    if (next.scale == 1) {
        discardCapture(api, &next);
        next.capture = next.color = next.depth = 0;
        result = set(context, parameter, size);
        if (!result)
            result = enable(context, EnableSurfaceBackingSize);
    }
    if (result)
        return result;
    if (STATE->context == context)
        discardCapture(api, STATE);
    *STATE = next;
    return 0;
}

void frameViewport(int x, int y, int w, int h, const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(viewport, void (*)(int, int, int, int))(x*scale, y*scale, w*scale, h*scale);
}

void frameScissor(int x, int y, int w, int h, const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(scissor, void (*)(int, int, int, int))(x*scale, y*scale, w*scale, h*scale);
}

void frameGetInteger(U32 parameter, int *values, const FrameAPI *api) {
    NATIVE(getInteger, void (*)(U32, int *))(parameter, values);
    if (parameter == Viewport && pixelScale(api) == 2) {
        values[0] /= 2;
        values[1] /= 2;
        values[2] /= 2;
        values[3] /= 2;
    }
}

void frameStorage(U32 target, U32 format, int w, int h, const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(storage, void (*)(U32, U32, int, int))(target, format, w*scale, h*scale);
}

void frameBlit(int sx, int sy, int sr, int sb, int dx, int dy, int dr, int db, U32 mask, U32 filter,
               const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(blit, void (*)(int, int, int, int, int, int, int, int, U32, U32))(
        sx*scale, sy*scale, sr*scale, sb*scale, dx*scale, dy*scale, dr*scale, db*scale, mask, filter);
}

void frameWindowPosition(int x, int y, const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(windowPosition, void (*)(int, int))(x*scale, y*scale);
}

void frameCopy(int x, int y, int w, int h, U32 type, const FrameAPI *api) {
    int scale = pixelScale(api);
    NATIVE(copy, void (*)(int, int, int, int, U32))(x*scale, y*scale, w*scale, h*scale, type);
}

void frameDraw(int w, int h, U32 format, U32 type, const void *pixels, const FrameAPI *api) {
    void (*draw)(int, int, U32, U32, const void *) = NATIVE(draw, void (*)(int, int, U32, U32, const void *));
    if (pixelScale(api) == 1) {
        draw(w, h, format, type, pixels);
        return;
    }
    NATIVE(pushAttrib, void (*)(U32))(PixelMode);
    const volatile float twice = 2.f;
    STATE->pixelZoom(twice, twice);
    draw(w, h, format, type, pixels);
    NATIVE(popAttrib, void (*)(void))();
}

void frameRead(int x, int y, int w, int h, U32 format, U32 type, void *pixels, const FrameAPI *api) {
    void (*read)(int, int, int, int, U32, U32, void *) =
        NATIVE(read, void (*)(int, int, int, int, U32, U32, void *));
    if (pixelScale(api) == 1 || w <= 0 || h <= 0) {
        read(x, y, w, h, format, type, pixels);
        return;
    }

    int oldRead, oldDraw;
    void (*getInteger)(U32, int *) = NATIVE(getInteger, void (*)(U32, int *));
    getInteger(ReadBinding, &oldRead);
    getInteger(DrawBinding, &oldDraw);
    int scissor = NATIVE(isEnabled, Byte (*)(U32))(ScissorTest);
    if (scissor)
        NATIVE(disable, void (*)(U32))(ScissorTest);
    void (*bind)(U32, U32) = NATIVE(bindFrame, void (*)(U32, U32));
    bind(DrawFramebuffer, STATE->capture);
    int depth = format == DepthComponent;
    NATIVE(blit, void (*)(int, int, int, int, int, int, int, int, U32, U32))(
        2*x, 2*y, 2*(x+w), 2*(y+h), 0, 0, w, h, depth ? DepthBuffer : ColorBuffer, depth ? Nearest : Linear);
    bind(DrawFramebuffer, oldDraw);
    bind(ReadFramebuffer, STATE->capture);
    read(0, 0, w, h, format, type, pixels);
    bind(ReadFramebuffer, oldRead);
    if (scissor)
        NATIVE(enable, void (*)(U32))(ScissorTest);
}
