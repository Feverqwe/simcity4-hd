// Build a position-independent i386 blob:
// clang -target i686-pc-windows-msvc -Os -ffreestanding -fno-stack-protector -fno-vectorize -fno-slp-vectorize -c ui_loading_preview.c -o preview.obj
// llvm-objcopy --dump-section .text=ui_loading_preview.i386 preview.obj preview-copy.obj
// For ui_region_tile.i386, compile the same source with -DREGION_TILE.
// The entry point is at offset zero. The .text section must have no relocations.

typedef unsigned int U32;
typedef unsigned char Byte;
typedef struct Buffer Buffer;
struct Buffer {
    void **methods;
};

#if defined(__i386__)
#define THISCALL __attribute__((thiscall))
#else
#define THISCALL
#endif

typedef void *(*GetFactory)(void);
#ifdef REGION_TILE
typedef Byte (THISCALL *CreateBuffer)(void *, Buffer **);
#else
typedef Byte (THISCALL *CreateBuffer)(void *, U32, U32, Buffer **);
#endif
typedef Byte (THISCALL *InitBuffer)(Buffer *, int, int, U32, U32);
typedef U32 (THISCALL *ReleaseBuffer)(Buffer *);
typedef Byte (THISCALL *LockBuffer)(Buffer *, U32);
typedef int (THISCALL *BufferInt)(Buffer *);
typedef U32 *(THISCALL *BufferPixels)(Buffer *);
typedef void (THISCALL *BufferFormat)(Buffer *, U32 *);

static inline U32 mix(U32 center, U32 neighbor) {
    const U32 lanes = 0x00ff00ff;
    U32 even = ((center & lanes) * 3 + (neighbor & lanes) + 0x00020002) >> 2;
    U32 odd = (((center >> 8) & lanes) * 3 + ((neighbor >> 8) & lanes) + 0x00020002) >> 2;
    return (even & lanes) | ((odd & lanes) << 8);
}

static __attribute__((always_inline)) inline Buffer *expand(Buffer *original, GetFactory getFactory) {
    const int width = ((BufferInt)original->methods[0x24 / 4])(original);
    const int height = ((BufferInt)original->methods[0x28 / 4])(original);
    if (width < 1 || height < 1 || width > 1024 || height > 1024) {
        return 0;
    }

    U32 format[2];
    ((BufferFormat)original->methods[0x3c / 4])(original, format);
    if (format[1] != 32 || (format[0] != 8 && format[0] != 9)) {
        return 0;
    }
    void *factory = getFactory();
    if (!factory) {
        return 0;
    }
    Buffer *enlarged = 0;
    const CreateBuffer create = (CreateBuffer)(*(void ***)factory)[0x0c / 4];
#ifdef REGION_TILE
    const Byte created = create(factory, &enlarged);
#else
    const Byte created = create(factory, 0xc470d325, 0x20732180, &enlarged);
#endif
    if (!created || !enlarged) {
        return 0;
    }
    if (!((InitBuffer)enlarged->methods[0x0c / 4])(enlarged, width * 2, height * 2, format[0], format[1])) {
        goto release;
    }
#ifdef REGION_TILE
    const U32 readFlags = 0x8040;
    const U32 writeFlags = 0x8040;
#else
    const U32 readFlags = 0x40;
    const U32 writeFlags = 0x8010;
#endif
    if (!((LockBuffer)original->methods[0x18 / 4])(original, readFlags)) {
        goto release;
    }
    if (!((LockBuffer)enlarged->methods[0x18 / 4])(enlarged, writeFlags)) {
        ((LockBuffer)original->methods[0x1c / 4])(original, readFlags);
        goto release;
    }

    const Byte *source = (const Byte *)((BufferPixels)original->methods[0x88 / 4])(original);
    Byte *destination = (Byte *)((BufferPixels)enlarged->methods[0x88 / 4])(enlarged);
    const int sourceStride = ((BufferInt)original->methods[0x8c / 4])(original);
    const int destinationStride = ((BufferInt)enlarged->methods[0x8c / 4])(enlarged);
    const int valid = source && destination && sourceStride >= width * 4 && destinationStride >= width * 8;
    if (valid) {
        // Sample at pixel centers. Apply the same weights to the premultiplied
        // color buffer and the separate coverage buffer to keep their edges aligned.
        for (int y = 0; y < height * 2; ++y) {
            const int row = y / 2;
            const U32 *line = (const U32 *)(source + row * sourceStride);
            U32 *out = (U32 *)(destination + y * destinationStride);
#ifdef REGION_TILE
            // Duplicate each sample exactly. The game derives hit intervals
            // and per-pixel overlay flags from these channels.
            for (int x = 0; x < width * 2; ++x) {
                out[x] = line[x / 2];
            }
#else
            int otherRow = row + ((y & 1) ? 1 : -1);
            if (otherRow < 0 || otherRow >= height) {
                otherRow = row;
            }
            const U32 *other = (const U32 *)(source + otherRow * sourceStride);
            for (int x = 0; x < width * 2; ++x) {
                const int column = x / 2;
                int otherColumn = column + ((x & 1) ? 1 : -1);
                if (otherColumn < 0 || otherColumn >= width) {
                    otherColumn = column;
                }
                out[x] = mix(mix(line[column], line[otherColumn]), mix(other[column], other[otherColumn]));
            }
#endif
        }
    }
    ((LockBuffer)enlarged->methods[0x1c / 4])(enlarged, writeFlags);
    ((LockBuffer)original->methods[0x1c / 4])(original, readFlags);
    if (valid) {
        return enlarged;
    }

release:
    ((ReleaseBuffer)enlarged->methods[0x08 / 4])(enlarged);
    return 0;
}

#ifdef REGION_TILE
void expandRegionTile(Buffer **planes, GetFactory getFactory) {
    Buffer *enlarged[4] = {0, 0, 0, 0};
    unsigned i;
    for (i = 0; i < 4; ++i) {
        if (planes[i]) {
            enlarged[i] = expand(planes[i], getFactory);
            if (!enlarged[i]) {
                for (unsigned j = 0; j < i; ++j) {
                    if (enlarged[j]) {
                        ((ReleaseBuffer)enlarged[j]->methods[0x08 / 4])(enlarged[j]);
                    }
                }
                return;
            }
        }
    }
    for (i = 0; i < 4; ++i) {
        if (enlarged[i]) {
            ((ReleaseBuffer)planes[i]->methods[0x08 / 4])(planes[i]);
            planes[i] = enlarged[i];
        }
    }
}
#else
void expandPreviewPair(Buffer **color, Buffer **coverage, GetFactory getFactory) {
    if (!color || !coverage || !*color || !*coverage || color == coverage || *color == *coverage) {
        return;
    }
    if (((BufferInt)(*color)->methods[0x24 / 4])(*color) != ((BufferInt)(*coverage)->methods[0x24 / 4])(*coverage) ||
        ((BufferInt)(*color)->methods[0x28 / 4])(*color) != ((BufferInt)(*coverage)->methods[0x28 / 4])(*coverage)) {
        return;
    }
    Buffer *enlargedColor = expand(*color, getFactory);
    if (!enlargedColor) {
        return;
    }
    Buffer *enlargedCoverage = expand(*coverage, getFactory);
    if (!enlargedCoverage) {
        ((ReleaseBuffer)enlargedColor->methods[0x08 / 4])(enlargedColor);
        return;
    }
    ((ReleaseBuffer)(*color)->methods[0x08 / 4])(*color);
    ((ReleaseBuffer)(*coverage)->methods[0x08 / 4])(*coverage);
    *color = enlargedColor;
    *coverage = enlargedCoverage;
}
#endif
