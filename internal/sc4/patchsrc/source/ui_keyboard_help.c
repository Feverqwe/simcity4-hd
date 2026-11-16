// Compile with clang -target i386-linux-gnu -march=i386 -Os -ffreestanding
// -fno-pic -fno-stack-protector, then extract the .text section.
typedef unsigned U32;
typedef unsigned char Byte;
#define TC __attribute__((thiscall))
#define METHOD(object, slot, type) ((type)(object)->methods[(slot) / 4])

typedef struct { void **methods; } Object;
typedef struct { U32 type, event, control; } Message;
typedef struct {
    void **methods;
    char *begin, *end, *limit;
    U32 resource[4];
} TextResource;

typedef Object *(TC *ChildByID)(Object *, U32);
typedef Byte (TC *FindChild)(Object *, U32, U32, Object **);
typedef Byte (TC *Visible)(Object *);
typedef void (TC *SetFlag)(Object *, U32, Byte);
typedef void (TC *SetMessageTarget)(Object *, Object *);
typedef void (TC *SetText)(Object *, const TextResource *, U32);
typedef void (TC *Release)(Object *);
typedef void (TC *LoadText)(TextResource *, U32, U32, U32);
typedef void (TC *CloseText)(TextResource *);

static void connectButton(Object *parent, U32 id, Object *target) {
    Object *button = METHOD(parent, 0x8c, ChildByID)(parent, id);
    if (button)
        METHOD(button, 0x158, SetMessageTarget)(button, target);
}

Byte keyboardHelpMessage(Object *root, const Message *event,
                         LoadText loadText, CloseText closeText,
                         const U32 *ids) {
    if (!event) {
        Object *panel = METHOD(root, 0x8c, ChildByID)(root, ids[1]);
        if (panel) {
            connectButton(root, ids[0], root);
            connectButton(panel, ids[3], root);
        }
        return 0;
    }
    const Byte button = event->type == 3 && event->event == 0x287259f6;
    const Byte escaping = event->type == 4 && (Byte)event->event == 27;
    if (!button && !escaping)
        return 0;

    Object *panel = METHOD(root, 0x8c, ChildByID)(root, ids[1]);
    if (!panel)
        return 0;
    const Byte visible = METHOD(panel, 0x11c, Visible)(panel);
    const Byte opening = button && event->control == ids[0] && !visible;
    if (!opening && !visible)
        return 0;

    // While the help dialog is visible, its controls own option messages.
    // In particular an Accept or Reset action must not affect preferences.
    if (!opening && !escaping && event->control != ids[3])
        return button;
    Object *options = METHOD(root, 0x8c, ChildByID)(root, 0x2a57db84);
    if (!options || (opening && !METHOD(options, 0x11c, Visible)(options)))
        return 1;

    if (opening) {
        Object *text = 0;
        if (!METHOD(panel, 0x94, FindChild)(panel, ids[2], 0x231a1c57, &text) || !text)
            return 1;
        TextResource help;
        loadText(&help, ids[4], ids[5], 0);
        const Byte present = help.begin != help.end;
        if (present)
            METHOD(text, 0xc0, SetText)(text, &help, 0);
        closeText(&help);
        METHOD(text, 0x08, Release)(text);
        if (!present)
            return 1;
    }

    METHOD(opening ? options : panel, 0x110, SetFlag)(opening ? options : panel, 1, 0);
    METHOD(opening ? panel : options, 0x110, SetFlag)(opening ? panel : options, 1, 1);
    return 1;
}
