// Build for i386-linux-gnu or x86_64-linux-gnu (-DASPYR).
// Use -Os -ffreestanding -fno-pic -fno-stack-protector
// -fno-asynchronous-unwind-tables -mgeneral-regs-only. Extract each
// relocation-free .text.catalog_* section into its own embedded payload.
typedef unsigned char Byte;
typedef unsigned int U32;
typedef struct { void **vt; } Object;
typedef struct { int l, t, r, b; } Rect;

#ifdef ASPYR
#define TC
#define SLOT(win, mac) (mac / 8)
#define BUTTON_FLAGS 0x15b
#define CALLBACKS
#define STRETCH_ARG , Blt stretch
#define TILE_ARGS , Blt stretch, Tile stock
#else
#define TC __attribute__((thiscall))
#define SLOT(win, mac) (win / 4)
#define BUTTON_FLAGS 0xdf
#define CALLBACKS Blt stretch,
#define STRETCH_ARG
#define TILE_ARGS
#endif
#define METHOD(obj, type, win, mac) ((type)(obj)->vt[SLOT(win, mac)])
#define ENTRY(name) __attribute__((section(".text.catalog_" #name)))

typedef int (TC *Size)(Object *);
typedef const Rect *(TC *Bounds)(Object *);
typedef Byte (TC *Lock)(Object *, U32);
typedef Byte (TC *Key)(Object *, U32 *);
typedef Byte (TC *Query)(Object *, U32, Object **);
typedef void (TC *Release)(Object *);
typedef U32 (TC *GetPixel)(Object *, int, int);
typedef void (TC *SetPixel)(Object *, int, int, U32);
typedef void (TC *Decode)(Object *, U32, Byte *, Byte *, Byte *, Byte *);
typedef U32 (TC *Encode)(Object *, Byte, Byte, Byte, Byte);
typedef Byte (TC *Copy)(Object *, Object *, const Rect *, const Rect *, const Rect *);
typedef Byte (*Blt)(Object *, Object *, const Rect *, const Rect *, const Rect *);
typedef Byte (*Tile)(Object *, Object *, const Rect *, const Rect *, int, int, const Rect *);
typedef Byte (TC *Style)(Object *, int, Object *, const Rect *);

static inline __attribute__((always_inline)) int isAtlas(Object *source, int w, int h) {
    return METHOD(source, Size, 0x24, 0x58)(source) == w &&
           METHOD(source, Size, 0x28, 0x60)(source) == h;
}

static inline __attribute__((always_inline)) int isFrameAtlas(Object *source) {
    const int width = METHOD(source, Size, 0x24, 0x58)(source);
    const int height = METHOD(source, Size, 0x28, 0x60)(source);
    return (width == 1168 && height == 212) || (width == 1224 && height == 248);
}

static inline int min(int a, int b) { return a < b ? a : b; }
static inline int max(int a, int b) { return a > b ? a : b; }

static inline __attribute__((always_inline)) Byte draw(
    Blt stretch, Object *dst, Object *src, const Rect *from, const Rect *to, const Rect *clip) {
    if (to->r <= to->l || to->b <= to->t || from->r <= from->l || from->b <= from->t)
        return 0;

    const Rect physical = {2 * from->l, 2 * from->t, 2 * from->r, 2 * from->b};
    U32 key;
    if (!METHOD(src, Key, 0x68, 0xe0)(src, &key))
        return stretch(dst, src, &physical, to, clip);
    Object *context = 0;
    if (METHOD(dst, Query, 0, 0)(dst, 0xab300b2b, &context) && context) {
        METHOD(context, Release, 0x08, 0x10)(context);
        return stretch(dst, src, &physical, to, clip);
    }

    // The game's software stretch copies the key color as an ordinary pixel.
    // Keep the destination under keyed pixels, as the original Blt does.
    const Rect *bounds = METHOD(dst, Bounds, 0x30, 0x68)(dst);
    int l = max(to->l, bounds->l), t = max(to->t, bounds->t);
    int r = min(min(to->r, bounds->r), to->l + from->r - from->l);
    int b = min(min(to->b, bounds->b), to->t + from->b - from->t);
    if (clip) {
        l = max(l, clip->l); t = max(t, clip->t);
        r = min(r, clip->r); b = min(b, clip->b);
    }
    if (l >= r || t >= b)
        return 0;
    if (!METHOD(src, Lock, 0x18, 0x40)(src, 0x800))
        return 0;
    for (int y = t; y < b; ++y) {
        const int sy = physical.t + 2 * (y - to->t) + 1;
        for (int x = l; x < r; ++x) {
            const int sx = physical.l + 2 * (x - to->l) + 1;
            const U32 pixel = METHOD(src, GetPixel, 0x54, 0xb8)(src, sx, sy);
            if (pixel != key) {
                Byte red, green, blue, alpha;
                METHOD(src, Decode, 0x84, 0x118)(src, pixel, &red, &green, &blue, &alpha);
                const U32 value = METHOD(dst, Encode, 0x80, 0x110)(dst, red, green, blue, alpha);
                METHOD(dst, SetPixel, 0x58, 0xc0)(dst, x, y, value);
            }
        }
    }
    METHOD(src, Lock, 0x1c, 0x48)(src, 0x800);
    return 1;
}

ENTRY(paint) Byte catalogPaint(CALLBACKS Object *dst, Object *src, const Rect *from, const Rect *to,
                              const Rect *clip STRETCH_ARG) {
    // These hooks see only catalog item sheets or the menu's frame atlas.
    // Match both dimensions so stock, x2 and custom-sized art retain their draw.
    if ((from->r - from->l == 88 && from->b - from->t == 88 && isAtlas(src, 704, 176)) ||
        isFrameAtlas(src))
        return draw(stretch, dst, src, from, to, clip);
    return METHOD(dst, Copy, 0x74, 0xf8)(dst, src, from, to, clip);
}

ENTRY(tile) Byte catalogTile(
#ifndef ASPYR
    Blt stretch, Tile stock,
#endif
    Object *dst, Object *src, const Rect *from, const Rect *to TILE_ARGS) {
    if (!isFrameAtlas(src))
        return stock(dst, src, from, to, 0, 0, 0);

    // Repeat at the original logical pitch, including the final partial tile.
    const int w = from->r - from->l, h = from->b - from->t;
    if (w <= 0 || h <= 0)
        return 0;
    for (int y = to->t; y < to->b;) {
        const int th = min(h, to->b - y);
        for (int x = to->l; x < to->r;) {
            const int tw = min(w, to->r - x);
            const Rect sample = {from->l, from->t, from->l + tw, from->t + th};
            const Rect cell = {x, y, x + tw, y + th};
            draw(stretch, dst, src, &sample, &cell, 0);
            x += tw;
        }
        y += th;
    }
    return 1;
}

ENTRY(height) int TC catalogHeight(Object *src) {
    const int h = METHOD(src, Size, 0x28, 0x60)(src);
    // SetBackgroundImage derives the middle slice and frame geometry
    // from the atlas height.
    const int w = METHOD(src, Size, 0x24, 0x58)(src);
    return ((w == 1168 && h == 212) || (w == 1224 && h == 248)) ? h / 2 : h;
}

ENTRY(button) Byte TC catalogButton(Object *button, int style, Object *src, const Rect *area) {
    // Bridge entries and the HUD restore button assign the atlas at runtime.
    // Supply the logical extent before SetStyle divides it into states.
    Rect logical;
    const int width = src ? METHOD(src, Size, 0x24, 0x58)(src) : 0;
    const int height = src ? METHOD(src, Size, 0x28, 0x60)(src) : 0;
    if (!area && ((width == 1424 && height == 232) || (width == 336 && height == 76))) {
        ((Byte *)button)[BUTTON_FLAGS] |= 0x80;
        logical.l = logical.t = 0;
        logical.r = width / 2;
        logical.b = height / 2;
        area = &logical;
    } else {
        ((Byte *)button)[BUTTON_FLAGS] &= 0x7f;
    }
    return METHOD(button, Style, 0x30, 0x60)(button, style, src, area);
}
