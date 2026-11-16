/* The stock clipped draw tiles the source across the destination. */
struct Rect { int left, top, right, bottom; };
#ifndef FRAME_SOURCE_SCALE
#define FRAME_SOURCE_SCALE 2
#endif
typedef void (__attribute__((thiscall)) *Begin)(void *, void *, float, float, float, float);
typedef void (__attribute__((thiscall)) *SetRect)(void *, const struct Rect *);
typedef void (__attribute__((thiscall)) *End)(void *);

void __attribute__((thiscall)) tileFrame(void *port, void *bitmap,
    const struct Rect *source, const struct Rect *destination, int x, int y) {
    if (source->right <= source->left || source->bottom <= source->top ||
        destination->right <= destination->left || destination->bottom <= destination->top)
        return;
    void **vt = *(void ***)port;
    const int scale = FRAME_SOURCE_SCALE;
    const struct Rect physical = {scale * source->left, scale * source->top,
                                  scale * source->right, scale * source->bottom};
    ((Begin)vt[0x74/4])(port, bitmap, (float)(destination->left+x), (float)(destination->top+y),
                         1.0f / scale, 1.0f / scale);
    ((SetRect)vt[0x7c/4])(port, &physical);
    ((SetRect)vt[0x8c/4])(port, destination);
    ((End)vt[0x78/4])(port);
    ((End)vt[0x70/4])(port);
}
