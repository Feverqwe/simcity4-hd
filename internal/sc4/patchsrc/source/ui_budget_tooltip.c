// clang -target i386-linux-gnu -march=i386 -Os -ffreestanding -fno-pic -fno-stack-protector -c ui_budget_tooltip.c -o budget-tooltip.o
// llvm-objcopy -O binary -j .text budget-tooltip.o ui_budget_tooltip.i386
typedef unsigned U32;
typedef unsigned char Byte;
#define TC __attribute__((thiscall))
#define METHOD(object, slot, type) ((type)(object)->methods[(slot) / 4])

typedef struct { void **methods; } Object;
typedef struct { int left, top, right, bottom; } Rect;
typedef struct {
    void **methods;
    char *begin, *end, *limit;
    U32 resource[4];
} TextResource;

typedef Byte (TC *FindChild)(Object *, U32, U32, Object **);
typedef Object *(TC *ChildByID)(Object *, U32);
typedef void (TC *Release)(Object *);
typedef Object *(TC *GetWindow)(Object *);
typedef const Rect *(TC *GetArea)(Object *);
typedef void (TC *SetArea)(Object *, int, int, int, int);
typedef Byte (TC *SetFlag)(Object *, U32, Byte);
typedef void (TC *SetWindowFlag)(Object *, U32, Byte);
typedef Byte (TC *SetStyle)(Object *, U32, Object *, const void *);
typedef Byte (TC *SetColor)(Object *, U32, U32);
typedef Byte (TC *SetTipFlags)(Object *, U32);
typedef Byte (TC *SetTooltip)(Object *, const TextResource *);
typedef void (TC *LoadText)(TextResource *, U32, U32, U32);
typedef void (TC *CloseText)(TextResource *);

static void fitCaptionAreas(Object *parent, Object *heading, Object *window) {
    const Rect *area = METHOD(window, 0xc0, GetArea)(window);
    // The button caches its measured caption width after the HD font pass.
    const int inkWidth = ((const unsigned short *)heading)[0xf8 / 2];
    const int inset = (area->right - area->left - inkWidth - 4) / 2;
    if (!inkWidth || inset <= 0)
        return;
    const int left = area->left + inset;
    METHOD(window, 0xdc, SetArea)(window, left, area->top, area->right - inset, area->bottom);

    // Funding is centered and comes first in the hit test. Trim the unused
    // space on both sides equally so its caption keeps the same center.
    Object *funding = METHOD(parent, 0x8c, ChildByID)(parent, 0x0abcde02);
    if (funding) {
        const Rect *fundingArea = METHOD(funding, 0xc0, GetArea)(funding);
        const int overlap = fundingArea->right - left;
        if (overlap > 0 && overlap * 2 < fundingArea->right - fundingArea->left)
            METHOD(funding, 0xdc, SetArea)(funding, fundingArea->left + overlap,
                                          fundingArea->top, left, fundingArea->bottom);
    }
}

void addBudgetTooltip(Object *parent, U32 service, U32 tooltipGroup,
                      LoadText loadText, CloseText closeText) {
    U32 text;
    switch (service) {
    case 0x4a5654ba: text = 0x0c04a37a; break; // School buses
    case 0xea56549e: text = 0x6c04a36e; break; // Ambulances
    default: return;
    }
    Object *heading = 0;
    if (!parent || !METHOD(parent, 0x94, FindChild)(parent, 0x0abcde03, 0x8810, &heading) || !heading)
        return;

    // The budget's own constructor supplied the caption, font and area.
    // Use a label face so the visible heading itself receives mouse enter.
    METHOD(heading, 0x30, SetStyle)(heading, 1, 0, 0);
    Object *window = METHOD(heading, 0x0c, GetWindow)(heading);
    // The native constructor set the normal caption color. Use it in the disabled state.
    METHOD(heading, 0x5c, SetColor)(heading, 4, ((const U32 *)heading)[0x110 / 4]);
    METHOD(window, 0x110, SetWindowFlag)(window, 0x2, 0); // Hover without a button action.
    METHOD(heading, 0x14, SetFlag)(heading, 0x200, 1); // Show on mouse enter.
    METHOD(heading, 0x14, SetFlag)(heading, 0x400, 0); // No tooltip timeout.
    TextResource tip;
    loadText(&tip, tooltipGroup, text, 0); // SetTooltip copies this temporary resource.
    if (tip.begin != tip.end) {
        METHOD(heading, 0x98, SetTipFlags)(heading, 0x01000000);
        METHOD(heading, 0x8c, SetTooltip)(heading, &tip);
        fitCaptionAreas(parent, heading, window);
    }
    closeText(&tip);
    METHOD(heading, 0x08, Release)(heading);
}
