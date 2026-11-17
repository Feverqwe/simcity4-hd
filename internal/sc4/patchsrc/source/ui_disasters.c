// Build for i386-linux-gnu or x86_64-linux-gnu (-DASPYR) with
// -Os -ffreestanding -fno-pic -fno-stack-protector
// -fno-asynchronous-unwind-tables -mgeneral-regs-only. Extract .text
// with llvm-objcopy -O binary -j .text and verify it has no relocations.
typedef unsigned int U32;
typedef unsigned char Byte;

#ifdef ASPYR
#define TC
enum { ADDREF = 0x08 / 8, RELEASE = 0x10 / 8, WIDTH = 0x58 / 8, HEIGHT = 0x60 / 8 };
#else
#define TC __attribute__((thiscall))
enum { ADDREF = 0x04 / 4, RELEASE = 0x08 / 4, WIDTH = 0x24 / 4, HEIGHT = 0x28 / 4 };
#endif

typedef struct { void **methods; } Buffer;
typedef struct {
    void **methods;
    Buffer *bitmap;
    U32 type, group, instance;
#ifdef ASPYR
    Byte loaderState[0x50 - 0x1c];
#else
    U32 loaderState[7];
#endif
} BitmapResource;

typedef void (TC *Load)(BitmapResource *, const U32 *, U32, U32);
typedef void (TC *Close)(BitmapResource *);
typedef void (TC *Ref)(Buffer *);
typedef int (TC *Size)(Buffer *);

void prepareDisasterBackground(BitmapResource *fallback, U32 denseInstance, Load load, Close close) {
    // The secondary menu also receives backgrounds with other resource keys.
    if (!fallback->bitmap || fallback->type != 0x856ddbac || fallback->group != 0x46a006b0 ||
        fallback->instance != 0xe74a40c3)
        return;

    const U32 key[3] = {fallback->type, fallback->group, denseInstance};
    BitmapResource dense;
    load(&dense, key, 1, 0);
    Buffer *source = dense.bitmap;
    if (source && ((Size)source->methods[WIDTH])(source) == 1224 &&
                  ((Size)source->methods[HEIGHT])(source) == 248) {
        // The menu frame uses logical geometry and stretches each slice
        // from this buffer. Keep the loaded texture alive past the lookup.
        ((Ref)source->methods[ADDREF])(source);
        Buffer *old = fallback->bitmap;
        fallback->bitmap = source;
        ((Ref)old->methods[RELEASE])(old);
    }
    close(&dense);
}
