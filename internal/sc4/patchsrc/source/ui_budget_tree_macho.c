// Compile for x86_64-linux-gnu with -Os -ffreestanding -fno-stack-protector
// -fno-vectorize -fno-slp-vectorize -fno-jump-tables -fno-asynchronous-unwind-tables.
// Link with ld.lld --entry=budgetLedger and SECTIONS { .text 0 : { *(.text) *(.rodata*) } }.
// Extract .text from the linked ELF at offset zero. The game installs it at RVA 0xb9d800.
typedef unsigned U32;
typedef unsigned char Byte;
typedef struct { void **methods; } Object;
typedef struct { int left, top, right, bottom; } Rect;
typedef struct { unsigned long image; Object *fontManager, *controlAtlas; U32 controls; } Context;
typedef struct { void *vtable; Object *image; unsigned long storage[8]; } Buffer;
typedef Byte (*ChildVisitor)(Object *, U32, Object *, Context *);

#define METHOD(object, offset, type) ((type)(object)->methods[(offset) / 8])
#define GAME(ctx, rva, type) ((type)((ctx)->image + (rva)))

enum { BudgetBitmap = 0x40000000 };
enum { NoAtlas, OrdinanceCheckboxes, FundingSliders, DetailsButton };
static void resizeTree(Object *window, const Rect *area, Context *ctx, Byte root);
static void paintBitmap(Object *window, const Rect *source, const Rect *destination, int drawOffset);
static void projectDetailsButton(Object *parent, U32 id, const Context *ctx);

void budgetLedger(Object *window, const Rect *source, const Rect *destination, int widthOrDrawOffset, U32 controls) {
    if (source) {
        paintBitmap(window, source, destination, widthOrDrawOffset);
        return;
    }
    if (!window)
        return;

    unsigned long entry;
    __asm__("leaq .LbudgetLedgerEntry(%%rip), %0" : "=r"(entry));
    Context ctx = {entry - 0xb9d800, 0, 0, controls};
    if (controls == DetailsButton) {
        projectDetailsButton(window, widthOrDrawOffset, &ctx);
        return;
    }
    const Rect area = *METHOD(window, 0x178, const Rect *(*)(Object *))(window);
    if (area.right - area.left != widthOrDrawOffset)
        return;
    ctx.fontManager = GAME(&ctx, 0x127da0, Object *(*)(void))(); // RZ::WCM()
    Buffer atlas;
    if (controls != NoAtlas) {
        const U32 group = controls == OrdinanceCheckboxes ? 0xe74a40c8 : 0xe74a40c6;
        const U32 instance = controls == OrdinanceCheckboxes ? 0x144161ea : 0x46a006a7;
        GAME(&ctx, 0x043c72, void (*)(Buffer *, U32, U32, U32, int, Byte))(
            &atlas, 0x856ddbac, group, instance, 1, 0);
        ctx.controlAtlas = atlas.image;
    }
    resizeTree(window, &area, &ctx, 1);
    if (controls != NoAtlas)
        GAME(&ctx, 0x043df0, void (*)(Buffer *))(&atlas);
}

__asm__(".set .LbudgetLedgerEntry, budgetLedger");

static void projectDetailsButton(Object *parent, U32 id, const Context *ctx) {
    // The constructor can put the button inside an interactive row wrapper.
    Object *button = METHOD(parent, 0x118, Object *(*)(Object *, U32))(parent, id);
    Object *control = 0;
    if (!button || !METHOD(button, 0, Byte (*)(Object *, U32, Object **))(button, 0x8810, &control))
        return;
    Buffer atlas;
    GAME(ctx, 0x043c72, void (*)(Buffer *, U32, U32, U32, int, Byte))(
        &atlas, 0x856ddbac, 0xe74a40c8, 0x140155b7, 1, 0);
    if (atlas.image) {
        // Keep stock bounds until the ledger's regular geometry pass.
        const Rect area = *METHOD(button, 0x178, const Rect *(*)(Object *))(button);
        GAME(ctx, 0x28088a, Byte (*)(Object *, U32, Object *, const Rect *))(control, 2, atlas.image, 0);
        METHOD(button, 0x1b0, void (*)(Object *, int, int, int, int))(
            button, area.left, area.top, area.right, area.bottom);
    }
    GAME(ctx, 0x043df0, void (*)(Buffer *))(&atlas);
    METHOD(control, 0x10, void (*)(Object *))(control);
}

static Byte scaleChild(Object *parent, U32 id, Object *child, Context *ctx) {
    (void)parent;
    (void)id;
    const Rect area = *METHOD(child, 0x178, const Rect *(*)(Object *))(child);
    resizeTree(child, &area, ctx, 0);
    return 1;
}

static void scaleFont(Object *control, Object *current, Context *ctx,
                      void (*setFont)(Object *, Object *)) {
    if (!current || !ctx->fontManager)
        return;
    Object *(*getFont)(Object *, U32) = METHOD(ctx->fontManager, 0x28, Object *(*)(Object *, U32));
    // Keep the style choice made by the native control constructor.
    const U32 styles[][2] = {
        {0x4a809916, 0x346d92f4}, {0x4a809919, 0x60199608}, // General header and button.
        {0xea85d304, 0xd1eeddae}, {0xea85d305, 0x1d1329ca}, // Ledger header and light.
        {0xac0c7170, 0x5a259567}, {0xea85d306, 0x442327e3}, // Underlined and heavy.
    };
    for (U32 i = 0; i < sizeof(styles) / sizeof(styles[0]); ++i) {
        if (getFont(ctx->fontManager, styles[i][0]) == current) {
            Object *replacement = getFont(ctx->fontManager, styles[i][1]);
            if (replacement)
                setFont(control, replacement);
            break;
        }
    }
}

static void resizeTree(Object *window, const Rect *area, Context *ctx, Byte root) {
    METHOD(window, 0x100, Byte (*)(Object *, U32, ChildVisitor, Context *))(
        window, 0x22ba0121, scaleChild, ctx);
    const int width = area->right - area->left;
    const int height = area->bottom - area->top;
    const int left = root ? area->left - width / 2 : area->left * 2;
    const int top = root ? area->top - height / 2 : area->top * 2;
    Byte (*query)(Object *, U32, Object **) = METHOD(window, 0, Byte (*)(Object *, U32, Object **));
    Object *control = 0;
    if (ctx->controls == FundingSliders && ctx->controlAtlas && query(window, 0x21325207, &control)) {
        // SetImage can resize the control to its image height. Apply the
        // ledger bounds afterward, as the stock slider constructor does.
        METHOD(control, 0x60, Byte (*)(Object *, Object *, const Rect *))(control, ctx->controlAtlas, 0);
        METHOD(control, 0x10, void (*)(Object *))(control);
        control = 0;
    }
    METHOD(window, 0x1b0, Byte (*)(Object *, int, int, int, int))(
        window, left, top, left + 2 * width, top + 2 * height);

    if (query(window, 0x212cdc1f, &control)) {
        // The text interface sits after the base window, unlike the button interface.
        Object *font = *(Object **)((char *)control + 8);
        scaleFont(control, font, ctx, GAME(ctx, 0x27f07a, void (*)(Object *, Object *)));
    } else if (query(window, 0x8810, &control)) {
        Object *font = *(Object **)((char *)control + 0x1a0);
        scaleFont(control, font, ctx, GAME(ctx, 0x280af8, void (*)(Object *, Object *)));
        if (ctx->controls == OrdinanceCheckboxes && ctx->controlAtlas &&
            GAME(ctx, 0x28087e, U32 (*)(Object *))(control) == 4)
            GAME(ctx, 0x28088a, Byte (*)(Object *, U32, Object *, const Rect *))(
                control, 4, ctx->controlAtlas, 0);
    } else if (query(window, 0xc12cea13, &control)) {
        GAME(ctx, 0x330bca, void (*)(Object *, U32, Byte))(control, BudgetBitmap, 1);
    }
    if (control)
        METHOD(control, 0x10, void (*)(Object *))(control);
}

static void paintBitmap(Object *window, const Rect *source, const Rect *destination, int drawOffset) {
    int width = source->right - source->left;
    int height = source->bottom - source->top;
    if (width <= 0 || height <= 0)
        return;
    Object *draw = *(Object **)((char *)window + drawOffset);
    Object *image = *(Object **)((char *)window + 0x158);
    const Rect fill = {destination->left, destination->top,
                       destination->left + 2 * width, destination->top + 2 * height};
    const float x = (float)(fill.left - 2 * source->left);
    const float y = (float)(fill.top - 2 * source->top);
    METHOD(draw, 0xe8, void (*)(Object *, Object *, float, float, float, float))(draw, image, x, y, 2, 2); // SetBlt.
    METHOD(draw, 0xf0, void (*)(Object *))(draw); // ClearBltTiling.
    METHOD(draw, 0x118, void (*)(Object *, const Rect *))(draw, &fill); // Fill with the bitmap.
    METHOD(draw, 0xe0, void (*)(Object *))(draw); // ClearBlt.
}
