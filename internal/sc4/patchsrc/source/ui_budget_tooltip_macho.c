// Compile for x86_64-linux-gnu with -Os -ffreestanding -fno-stack-protector
// -fno-vectorize -fno-slp-vectorize -fno-jump-tables -fno-asynchronous-unwind-tables.
// Extract relocation-free .text. The entry is installed at RVA 0xb9d200.
typedef unsigned U32;
typedef unsigned char Byte;
typedef struct { void **methods; } Object;
typedef struct { unsigned long storage[5]; } String;
typedef struct {
    Byte unknown[0x38];
    U32 department;
} Budget;

typedef int (*AddText)(Budget *, Object *, U32, int, int, const String *, U32, U32, Byte, Byte, Byte);
typedef void (*AddButton)(Budget *, Object *, U32, int, int, int, const String *, U32, U32, Byte, Byte, Byte);
typedef Byte (*Query)(Object *, U32, U32, Object **);
typedef void (*Flag)(Object *, U32, Byte);
typedef void (*StringOp)(String *);
typedef U32 (*StringLength)(const String *);
typedef Byte (*LoadText)(String *, U32, U32, void *);
typedef Byte (*Style)(Object *, U32, Object *, const void *);
typedef Byte (*GetColor)(const Object *, U32, U32 *);
typedef Byte (*SetColor)(Object *, U32, U32);
typedef Byte (*SetTip)(Object *, const String *);
typedef Byte (*Placement)(Object *, U32);
typedef Byte (*Fit)(Object *);

#define GAME(rva, type) ((type)(base + (rva)))
#define METHOD(object, offset, type) ((type)(object)->methods[(offset) / 8])

int departmentHeading(Budget *budget, Object *parent, U32 id, int x, int y, const String *caption,
                      U32 align, U32 font, Byte red, Byte green, Byte blue) {
    unsigned long base;
    __asm__("leaq .LheadingEntry(%%rip), %0" : "=r"(base));
    base -= 0xb9d200;

    U32 text = 0;
    if (budget->department == 0x4a5654ba)
        text = 0x0c04a37a; // School buses.
    if (budget->department == 0xea56549e)
        text = 0x6c04a36e; // Ambulances.
    if (!text)
        return GAME(0x332570, AddText)(budget, parent, id, x, y, caption, align, font, red, green, blue);

    String tip;
    GAME(0x4878d0, StringOp)(&tip);
    GAME(0x120e10, LoadText)(&tip, 0xe74a40cb, text, 0);
    if (!GAME(0x1715c, StringLength)(&tip)) {
        GAME(0x05f070, StringOp)(&tip);
        return GAME(0x332570, AddText)(budget, parent, id, x, y, caption, align, font, red, green, blue);
    }

    // The stock button constructor supplies the font, caption, position and color.
    // The label face shrinks to the text so the hover target follows the heading.
    GAME(0x332970, AddButton)(budget, parent, id, x, y, 0, caption, 1, font, red, green, blue);
    Object *label = 0;
    if (METHOD(parent, 0x128, Query)(parent, id, 0x8810, &label) && label) {
        GAME(0x28088a, Style)(label, 1, 0, 0);
        U32 normalColor;
        if (GAME(0x282146, GetColor)(label, 0, &normalColor))
            GAME(0x2820cc, SetColor)(label, 4, normalColor);
        Object *window = (Object *)((char *)label + 8); // cIGZWin base of cGZWinBtn.
        METHOD(window, 0x220, Flag)(window, 2, 0);
        GAME(0x281da6, Fit)(label);
        GAME(0x280d68, Flag)(label, 0x200, 1); // Immediate tooltip.
        GAME(0x280d68, Flag)(label, 0x400, 0); // No timeout.
        GAME(0x282264, Placement)(label, 0x01000000);
        GAME(0x2821b4, SetTip)(label, &tip);
        METHOD(label, 0x10, void (*)(Object *))(label);
    }
    GAME(0x05f070, StringOp)(&tip);
    return 0;
}

__asm__(".set .LheadingEntry, departmentHeading");
