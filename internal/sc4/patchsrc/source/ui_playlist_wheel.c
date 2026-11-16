// clang -target i686-pc-windows-msvc -Os -ffreestanding -fno-stack-protector -c ui_playlist_wheel.c -o playlist.obj
// llvm-objcopy --dump-section .text=ui_playlist_wheel.i386 playlist.obj
// Aspyr: clang -target x86_64-linux-gnu -DSC4_ASPYR -Os -ffreestanding -fno-stack-protector -c ui_playlist_wheel.c -o playlist.o
// llvm-objcopy -O binary -j .text playlist.o ui_playlist_wheel.x86_64

typedef unsigned int U32;
typedef unsigned char Byte;
#ifdef SC4_ASPYR
#define THISCALL
enum { idSlot = 0x1f8, relativeScrollSlot = 0xf8, absoluteScrollSlot = 0x120,
       valueSlot = 0x28, rangeSlot = 0x40, scrollbarOffset = 0x358, topRowOffset = 0x1a8 };
#else
#define THISCALL __attribute__((thiscall))
enum { idSlot = 0xfc, relativeScrollSlot = 0x7c, absoluteScrollSlot = 0x90,
       valueSlot = 0x14, rangeSlot = 0x20, scrollbarOffset = 0x26c, topRowOffset = 0x12c };
#endif

#define METHOD(object, offset, type) ((type)(*(void ***)(object))[(offset) / sizeof(void *)])

typedef U32 (THISCALL *WindowID)(void *);
typedef Byte (THISCALL *ScrollRows)(void *, int);
typedef Byte (THISCALL *GetMinMax)(void *, int *, int *);

void THISCALL scrollPlaylist(void **grid, int rows) {
    void **window = grid + 1;
    if (METHOD(window, idSlot, WindowID)(window) == 0x8a550c56) {
        // The scrollbar already computes the valid top-row range. The
        // relative grid scroll discards the final stop if it leaves empty
        // pixels below the last full row.
        void *scrollbar = grid[scrollbarOffset / sizeof(void *)];
        int first = 0, last = -1;
        if (scrollbar && METHOD(scrollbar, rangeSlot, GetMinMax)(scrollbar, &first, &last) &&
            first >= 0 && last >= first) {
            const int current = *(int *)((char *)grid + topRowOffset);
            if (rows != 0 && first < last) {
                const long long requested = (long long)current + rows;
                const int target = requested < first ? first : requested > last ? last : (int)requested;
                METHOD(grid, absoluteScrollSlot, ScrollRows)(grid, target);
                // The grid setter only moves the rows. Push its actual
                // position to the scrollbar as the native grid refresh does.
                const int top = *(int *)((char *)grid + topRowOffset);
                METHOD(scrollbar, valueSlot, ScrollRows)(scrollbar, top);
            }
            return;
        }
    }
    METHOD(grid, relativeScrollSlot, ScrollRows)(grid, rows);
}
