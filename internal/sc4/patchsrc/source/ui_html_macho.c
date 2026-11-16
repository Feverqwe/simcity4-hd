// Compile for x86_64-linux-gnu with -Os -ffreestanding -fno-stack-protector
// -fno-vectorize -fno-slp-vectorize -fno-jump-tables -fno-asynchronous-unwind-tables.
// Extract relocation-free .text at offset zero. Aspyr installs it at RVA 0xb9e900.
typedef unsigned char Byte;
typedef struct { void **methods; } Object;
typedef int (*Dimension)(Object *);
typedef Object *(*AsWindow)(Object *);
typedef void (*SetArea)(Object *, int, int, int, int);
typedef struct { int left, top, right, bottom; } Rect;

#define METHOD(object, offset, type) ((type)(object)->methods[(offset) / 8])
#define GAME(rva, type) ((type)(base + (rva)))

enum { Layout, Resize, Reflow, Remove };
enum { BarWidth = 28, VerticalInset = 4 };

static int clamp(int value, int low, int high) {
    if (value < low)
        return low;
    return value > high ? high : value;
}

static void layoutBars(Object *html, unsigned long base) {
    Object *vertical = *(Object **)((char *)html + 0x288);
    Object *horizontal = *(Object **)((char *)html + 0x290);
    if (!vertical && !horizontal)
        return;

    int width = METHOD(html, 0x148, Dimension)(html);
    int height = METHOD(html, 0x150, Dimension)(html);
    width = width > 0 ? width : 0;
    height = height > 0 ? height : 0;
    const int joint = vertical ? clamp(width - BarWidth, 0, width) : width;

    if (vertical) {
        Object *window = METHOD(vertical, 0x18, AsWindow)(vertical);
        const int top = clamp(VerticalInset, 0, height);
        const int bottom = clamp(height - VerticalInset, top, height);
        METHOD(window, 0x1b0, SetArea)(window, joint, top, width, bottom);
    }
    if (horizontal) {
        Object *window = METHOD(horizontal, 0x18, AsWindow)(horizontal);
        Rect cell;
        Object *image;
        int imageFlags;
        const int barHeight = GAME(0x27b670, Byte (*)(Object *, Rect *, Object **, int *))(
            horizontal, &cell, &image, &imageFlags)
            ? cell.bottom - cell.top : METHOD(window, 0x150, Dimension)(window);
        const int top = height - clamp(barHeight, 0, height);
        METHOD(window, 0x1b0, SetArea)(window, 0, top, joint, height);
    }
}

Byte htmlScrollbars(Object *html, int operation) {
    unsigned long entry;
    __asm__("leaq .LhtmlScrollbars(%%rip), %0" : "=r"(entry));
    const unsigned long base = entry - 0xb9e900;
    if (operation == Reflow) {
        Byte result = GAME(0x5688e8, Byte (*)(Object *))(html);
        GAME(0x568100, void (*)(Object *, Byte))(html, 1);
        return result;
    }

    layoutBars(html, base);
    if (operation == Resize)
        return GAME(0x56718e, Byte (*)(Object *))(html);
    if (operation == Remove)
        METHOD(html, 0x2e0, void (*)(Object *))(html);
    return 1;
}

__asm__(".set .LhtmlScrollbars, htmlScrollbars");
