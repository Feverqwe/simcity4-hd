// Aspyr 185949, freestanding x86_64. Extract the linked .text as a single block.
// Compile with -target x86_64-linux-gnu -Os -ffreestanding -fno-pic
// -fno-stack-protector -fno-asynchronous-unwind-tables -fno-vectorize -fno-slp-vectorize.
// Extract relocation-free .text with llvm-objcopy.
typedef unsigned char Byte;
typedef struct {
    void *library, *methods;
    unsigned format;
    int advanceX, advanceY;
} Glyph;

typedef struct {
    Glyph glyph;
    int left, top;
    unsigned rows, width;
    int pitch;
    Byte *pixels;
    unsigned short grayLevels;
} BitmapGlyph;

typedef struct { int left, top, right, bottom; } Rect;
typedef struct {
    Rect destination;
    const Byte *pixels;
    int pitch, width, height;
    unsigned codepoint;
} GlyphInfo;

typedef struct {
    int stockCache, alloc, free, charIndex, load, get, transform, raster, release, fontMethods;
} FontAPI;

#define NATIVE(field, type) ((type)((const char *)api + api->field))
#define FIELD(object, offset, type) (*(type *)((char *)object + (offset)))
enum { DenseSource = 1u << 30, Outline = 0x6f75746c };

_Static_assert(sizeof(GlyphInfo) == 40, "Aspyr glyph input");
_Static_assert(__builtin_offsetof(BitmapGlyph, pixels) == 0x38, "Aspyr bitmap pixels");
_Static_assert(__builtin_offsetof(BitmapGlyph, grayLevels) == 0x40, "Aspyr bitmap coverage");

static int coverageBitmap(const BitmapGlyph *glyph) {
    return glyph->pixels && glyph->grayLevels == 256 && glyph->pitch > 0 &&
           glyph->width <= (unsigned)glyph->pitch && glyph->rows && glyph->width &&
           glyph->rows <= 4096 && glyph->pitch <= 4096;
}

static int alignDenseMask(const GlyphInfo *input, const BitmapGlyph *base, const BitmapGlyph *dense,
                          const Byte *coverageTable, Byte *output) {
    if (!coverageBitmap(base) || !coverageBitmap(dense) ||
        base->width != (unsigned)input->width || base->rows != (unsigned)input->height)
        return 0;
    // Match the grayscale adjustment used by cGZFont::CopyGlyphToSourceData.
    for (int y = 0; y < input->height; ++y)
        for (int x = 0; x < input->width; ++x) {
            Byte value = base->pixels[y * base->pitch + x];
            if ((coverageTable ? coverageTable[value] : value) != input->pixels[y * input->pitch + x])
                return 0;
        }
    const int dx = 2 * base->left - dense->left;
    const int dy = dense->top - 2 * base->top;
    for (int y = 0; y < 2 * input->height; ++y)
        for (int x = 0; x < 2 * input->width; ++x) {
            const unsigned sx = x + dx, sy = y + dy;
            Byte value = sx < dense->width && sy < dense->rows ? dense->pixels[sy * dense->pitch + sx] : 0;
            *output++ = coverageTable ? coverageTable[value] : value;
        }
    return 1;
}

static int rasterizeDense(const FontAPI *api, void *font, const GlyphInfo *input, Byte *output) {
    if (FIELD(font, 0, void *) != NATIVE(fontMethods, void *) || FIELD(font, 0x78, int) == 1)
        return 0;
    void **faceRef = FIELD(font, 0x180, void **);
    if (!faceRef || !*faceRef)
        return 0;
    void *face = *faceRef;
    unsigned index = NATIVE(charIndex, unsigned (*)(void *, unsigned))(face, input->codepoint);
    if (!index)
        index = FIELD(font, 0x17c, unsigned);
    if (!index || NATIVE(load, int (*)(void *, unsigned, int))(face, index, 0))
        return 0;

    const Byte *coverageTable = FIELD(font, 0x88, const Byte *);
    if (coverageTable == FIELD(font, 0x90, const Byte *))
        coverageTable = 0;
    void *slot = FIELD(face, 0x78, void *);
    int (*get)(void *, Glyph **) = NATIVE(get, int (*)(void *, Glyph **));
    int (*raster)(Glyph **, unsigned, const void *, Byte) =
        NATIVE(raster, int (*)(Glyph **, unsigned, const void *, Byte));
    void (*release)(Glyph *) = NATIVE(release, void (*)(Glyph *));
    Glyph *base = 0, *dense = 0;
    volatile int twice[4];
    twice[0] = twice[3] = 0x20000;
    twice[1] = twice[2] = 0;
    int result = 0;
    if (!get(slot, &base) && base->format == Outline && !get(slot, &dense) &&
        !raster(&base, 0, 0, 1) &&
        !NATIVE(transform, int (*)(Glyph *, const int *, const void *))(dense, (const int *)twice, 0) &&
        !raster(&dense, 0, 0, 1))
        result = alignDenseMask(input, (const BitmapGlyph *)base, (const BitmapGlyph *)dense, coverageTable, output);
    if (dense)
        release(dense);
    if (base)
        release(base);
    return result;
}

int *cacheRetinaGlyph(void *renderer, void *font, const GlyphInfo *input, Byte reset, const FontAPI *api) {
    int *(*cache)(void *, void *, const GlyphInfo *, Byte) =
        NATIVE(stockCache, int *(*)(void *, void *, const GlyphInfo *, Byte));
    void *atlas = FIELD(renderer, 0x18, void *);
    void **methods = FIELD(atlas, 0, void **);
    const int w = input->width, h = input->height;
    const int atlasWidth = ((int (*)(void *))methods[0x58/8])(atlas);
    const int atlasHeight = ((int (*)(void *))methods[0x60/8])(atlas);
    if (!input->pixels || w <= 0 || h <= 0 || w > 512 || h > 512 ||
        input->pitch < w || w > atlasWidth/2 || h > atlasHeight/2)
        return cache(renderer, font, input, reset);
    Byte *pixels = NATIVE(alloc, void *(*)(unsigned long))(4UL * w * h);
    if (!pixels)
        return cache(renderer, font, input, reset);
    int *position;
    if (rasterizeDense(api, font, input, pixels)) {
        GlyphInfo dense = *input;
        dense.pixels = pixels;
        dense.pitch = dense.width = 2 * w;
        dense.height = 2 * h;
        position = cache(renderer, font, &dense, reset);
        if (position)
            // The two-coordinate cache value is private to DrawChars.
            // Its blit removes this flag before using the atlas rectangle.
            position[0] |= DenseSource;
    } else {
        position = cache(renderer, font, input, reset);
    }
    NATIVE(free, void (*)(void *))(pixels);
    return position;
}

void blitRetinaGlyph(void *context, void *atlas, const Rect *source, const Rect *destination) {
    void **methods = FIELD(context, 0, void **);
    void (*blit)(void *, void *, const Rect *, const Rect *) =
        (void (*)(void *, void *, const Rect *, const Rect *))methods[0x130/8];
    if (!(source->left & DenseSource)) {
        blit(context, atlas, source, destination);
        return;
    }
    // The atlas cell is twice as large, but the fill still uses the
    // original logical rectangle. SetBlt takes the origin of the entire
    // texture after the source-to-destination scale has been applied.
    const volatile float half = 0.5f;
    const float scale = half;
    const float x = destination->left - scale * (source->left & ~DenseSource);
    const float y = destination->top - scale * source->top;
    ((void (*)(void *, void *, float, float, float, float))methods[0xe8/8])(
        context, atlas, x, y, scale, scale);
    ((void (*)(void *))methods[0xf0/8])(context);
    ((void (*)(void *, const Rect *))methods[0x118/8])(context, destination);
    ((void (*)(void *))methods[0xe0/8])(context);
}
