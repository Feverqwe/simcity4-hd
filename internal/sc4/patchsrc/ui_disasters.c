// clang -target i386-linux-gnu -march=i386 -Os -ffreestanding -fno-pic -fno-stack-protector -c ui_disasters.c -o /tmp/sc4-disasters.o
// llvm-objcopy -O binary -j .text /tmp/sc4-disasters.o ui_disasters.i386
// The entry point is at offset zero. The .text section must have no relocations.
typedef unsigned int U32;
typedef unsigned char Byte;
#define THISCALL __attribute__((thiscall))

typedef struct { void **methods; } Buffer;
typedef struct {
    void **methods;
    Buffer *bitmap;
    U32 type, group, instance;
    U32 loaderState[7];
} BitmapResource;

typedef void (THISCALL *Load)(BitmapResource *, const U32 *, U32, U32);
typedef void (THISCALL *Close)(BitmapResource *);
typedef void *(*Factory)(void);
typedef Byte (THISCALL *Create)(void *, U32, U32, Buffer **);
typedef Byte (THISCALL *Init)(Buffer *, int, int, U32, U32);
typedef void (THISCALL *Release)(Buffer *);
typedef Byte (THISCALL *Lock)(Buffer *, U32);
typedef int (THISCALL *Measure)(Buffer *);
typedef void *(THISCALL *Pixels)(Buffer *);
typedef void (THISCALL *Format)(Buffer *, U32 *);

static inline void release(Buffer *buffer) {
    ((Release)buffer->methods[0x08 / 4])(buffer);
}

static inline U32 filter(const U32 *top, const U32 *bottom, int hasAlpha) {
    U32 sample[4] = {top[0], top[1], bottom[0], bottom[1]};
    U32 alpha = 0, red = 0, green = 0, blue = 0;
    for (int i = 0; i < 4; ++i) {
        const U32 value = sample[i];
        const U32 weight = hasAlpha ? value >> 24 : 255;
        alpha += weight;
        red += ((value >> 16) & 255) * weight;
        green += ((value >> 8) & 255) * weight;
        blue += (value & 255) * weight;
    }
    if (!alpha)
        return 0;
    return ((alpha + 2) / 4 << 24) |
           ((red + alpha / 2) / alpha << 16) |
           ((green + alpha / 2) / alpha << 8) |
           ((blue + alpha / 2) / alpha);
}

static __attribute__((always_inline)) inline Buffer *project(Buffer *source, Buffer *fallback, Factory factory) {
    const int width = ((Measure)fallback->methods[0x24 / 4])(fallback);
    const int height = ((Measure)fallback->methods[0x28 / 4])(fallback);
    if (width <= 0 || height <= 0 || width > 2048 || height > 2048 ||
        ((Measure)source->methods[0x24 / 4])(source) != 2 * width ||
        ((Measure)source->methods[0x28 / 4])(source) != 2 * height)
        return 0;

    U32 format[2];
    ((Format)source->methods[0x3c / 4])(source, format);
    if (format[1] != 32 || (format[0] != 8 && format[0] != 9))
        return 0;
    void *owner = factory();
    if (!owner)
        return 0;
    Buffer *result = 0;
    if (!((Create)(*(void ***)owner)[0x0c / 4])(owner, 0xc470d325, 0x20732180, &result) || !result)
        goto failed;
    if (!((Init)result->methods[0x0c / 4])(result, width, height, format[0], format[1]))
        goto failed;
    const U32 read = 0x40, write = 0x8010;
    if (!((Lock)source->methods[0x18 / 4])(source, read))
        goto failed;
    if (!((Lock)result->methods[0x18 / 4])(result, write)) {
        ((Lock)source->methods[0x1c / 4])(source, read);
        goto failed;
    }
    const Byte *input = ((Pixels)source->methods[0x88 / 4])(source);
    Byte *output = ((Pixels)result->methods[0x88 / 4])(result);
    const int inputStride = ((Measure)source->methods[0x8c / 4])(source);
    const int outputStride = ((Measure)result->methods[0x8c / 4])(result);
    const int valid = input && output && inputStride >= width * 8 && outputStride >= width * 4 &&
                      !(inputStride & 3) && !(outputStride & 3);
    if (valid) {
        for (int y = 0; y < height; ++y) {
            const U32 *top = (const U32 *)(input + 2 * y * inputStride);
            const U32 *bottom = (const U32 *)((const Byte *)top + inputStride);
            U32 *row = (U32 *)(output + y * outputStride);
            for (int x = 0; x < width; ++x)
                row[x] = filter(top + 2 * x, bottom + 2 * x, format[0] == 9);
        }
    }
    ((Lock)result->methods[0x1c / 4])(result, write);
    ((Lock)source->methods[0x1c / 4])(source, read);
    if (valid)
        return result;

failed:
    if (result)
        release(result);
    return 0;
}

// The menu composes the background slices with a 1:1 blit. Give that
// compositor a buffer at its own density, retaining the packaged fallback
// until the entire dense image has been sampled successfully.
void prepareDisasterBackground(BitmapResource *fallback, U32 denseInstance, Load load, Close close, Factory factory) {
    if (!fallback->bitmap)
        return;
    const U32 key[3] = {fallback->type, fallback->group, denseInstance};
    BitmapResource dense;
    load(&dense, key, 1, 0);
    Buffer *result = dense.bitmap ? project(dense.bitmap, fallback->bitmap, factory) : 0;
    close(&dense);
    if (result) {
        release(fallback->bitmap);
        fallback->bitmap = result;
    }
}
