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
typedef Byte (TC *Create)(Object *, U32, U32, Object **);
typedef void (TC *Release)(Object *);
typedef Object *(TC *GetWindow)(Object *);
typedef const Rect *(TC *GetArea)(Object *);
typedef void (TC *SetArea)(Object *, int, int, int, int);
typedef void (TC *SetFlag)(Object *, U32, Byte);
typedef Byte (TC *SetTooltip)(Object *, const TextResource *);
typedef void (TC *AddChild)(Object *, Object *);
typedef void (TC *LoadText)(TextResource *, U32, U32, U32);
typedef void (TC *CloseText)(TextResource *);
typedef Object *(*GetFactory)(void);

// Called after the text constructor and before the budget tree doubles
// every control. The caption owns an ordinary tooltip-only child.
void addBudgetTooltip(Object *parent, U32 service, U32 tooltipGroup,
                      GetFactory getFactory, LoadText loadText, CloseText closeText) {
    U32 text;
    switch (service) {
    case 0x4a5654ba: text = 0x0c04a37a; break; // School buses
    case 0xea56549e: text = 0x6c04a36e; break; // Ambulances
    default: return;
    }
    Object *caption = 0;
    if (!parent || !METHOD(parent, 0x94, FindChild)(parent, 0x0abcde05, 0x22ba0121, &caption) || !caption)
        return;

    const Rect *area = METHOD(caption, 0xc0, GetArea)(caption);
    const int width = area->right - area->left, height = area->bottom - area->top;
    if (width > 0 && height > 0) {
        TextResource tip;
        loadText(&tip, tooltipGroup, text, 1);
        if (tip.begin != tip.end) {
            Object *factory = getFactory();
            Object *button = 0;
            if (factory && METHOD(factory, 0x0c, Create)(factory, 0x22ecfc47, 0x8810, &button) && button) {
                Object *window = METHOD(button, 0x0c, GetWindow)(button);
                METHOD(button, 0x14, SetFlag)(button, 0x10, 1); // Skip the fallback button face.
                METHOD(window, 0x110, SetFlag)(window, 0x2, 0); // Mouse movement only.
                METHOD(window, 0x110, SetFlag)(window, 0x8000, 0); // No keyboard focus.
                METHOD(window, 0x110, SetFlag)(window, 0x8, 0); // No background color.
                if (METHOD(button, 0x8c, SetTooltip)(button, &tip)) {
                    // Restrict the hover area to the right-hand column.
                    const int left = width > 110 ? width - 110 : 0;
                    METHOD(window, 0xdc, SetArea)(window, left, 0, width, height);
                    METHOD(caption, 0x38, AddChild)(caption, window);
                }
                METHOD(button, 0x08, Release)(button);
            }
        }
        closeText(&tip);
    }
    METHOD(caption, 0x08, Release)(caption);
}
