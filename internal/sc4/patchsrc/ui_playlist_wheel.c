// clang -target i686-pc-windows-msvc -Os -ffreestanding -fno-stack-protector -c ui_playlist_wheel.c -o playlist.obj
// llvm-objcopy --dump-section .text=ui_playlist_wheel.i386 playlist.obj

typedef unsigned int U32;
typedef unsigned char Byte;
#define THISCALL __attribute__((thiscall))

typedef U32 (THISCALL *WindowID)(void *);
typedef Byte (THISCALL *ScrollRows)(void *, int);
typedef Byte (THISCALL *GetMinMax)(void *, int *, int *);

void THISCALL scrollPlaylist(void **grid, int rows) {
    void **window = grid + 1;
    if (((WindowID)(*(void ***)window)[0xfc / 4])(window) == 0x8a550c56) {
        // The scrollbar already computes the valid top-row range. The
        // relative grid scroll discards the final stop if it leaves empty
        // pixels below the last full row.
        void *scrollbar = grid[0x26c / 4];
        int first = 0, last = -1;
        if (scrollbar && ((GetMinMax)(*(void ***)scrollbar)[0x20 / 4])(scrollbar, &first, &last) &&
            first >= 0 && last >= first) {
            const int current = *(int *)((char *)grid + 0x12c);
            if (rows != 0 && first < last) {
                const long long requested = (long long)current + rows;
                const int target = requested < first ? first : requested > last ? last : (int)requested;
                ((ScrollRows)(*(void ***)grid)[0x90 / 4])(grid, target);
                // The grid setter only moves the rows. Push its actual
                // position to the scrollbar as the native grid refresh does.
                const int top = *(int *)((char *)grid + 0x12c);
                ((ScrollRows)(*(void ***)scrollbar)[0x14 / 4])(scrollbar, top);
            }
            return;
        }
    }
    ((ScrollRows)(*(void ***)grid)[0x7c / 4])(grid, rows);
}
