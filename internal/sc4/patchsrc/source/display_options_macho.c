// Compile for x86_64-linux-gnu with -Os -ffreestanding -fno-stack-protector
// -fno-vectorize -fno-slp-vectorize -fno-jump-tables -fno-asynchronous-unwind-tables.
// Extract relocation-free .text. displayOptions is the entry at offset zero.
typedef unsigned U32;
typedef unsigned char Byte;
typedef unsigned long long U64;
typedef struct { void **methods; } Object;
typedef struct { Object object; Object *host; } Dialog;
typedef struct { U64 storage[7]; } String;
typedef struct { U32 w, h; } Size;
typedef struct { U32 mode; Size size; } Settings;

enum { Open, Defaults, Apply, ModeChanged, Startup, PrepareWindow };
enum { Automatic, Windowed, Fullscreen, Borderless };
enum { ResolutionID=0xe74a4401, DesktopID, ErrorID, ModeID, ModeTitleID,
       ResolutionFrameID, ModeFrameID, RestartID, ComboID=0x412ce496 };

typedef struct {
    int get, put, sync, windowed, hint, getenv, numModes, getMode, currentMode, inWindow,
        localizedString, destroyLocalized, string, emptyString, destroyString, toChar,
        displayID, mainDisplayID, copyDisplayModes, arrayCount, arrayValue, release,
        pixelWidth, pixelHeight, getWindow, setWindowData, getWindowData, copyDesktopMode;
    char key[16], spacesHint[32], enableSpaces[2], disableSpaces[2], resolutionVar[24];
    char pendingKey[24], failedKey[24];
} Context;

#define GAME(field, type) ((type)((const char *)ctx + ctx->field))
#define METHOD(object, offset, type) ((type)(object)->methods[(offset)/8])

typedef Object *(*Window)(Object *);
typedef Object *(*Child)(Object *, U32);
typedef Byte (*Query)(Object *, U32, U32, Object **);
typedef void (*Flag)(Object *, U32, Byte);
typedef void (*Target)(Object *, Object *);
typedef void (*Release)(Object *);
typedef void (*Clear)(Object *);
typedef Byte (*Insert)(Object *, const String *, int);
typedef Byte (*Select)(Object *, int, Byte);
typedef int (*Selection)(Object *);
typedef Byte (*GetString)(Object *, int, String *);
typedef void (*TextInit)(String *, const char *);
typedef void (*LocaleInit)(String *, U32, U32, Byte);
typedef void (*StringOp)(String *);
typedef const char *(*Text)(const String *);

static U64 startup(const Context *ctx);
static void prepareWindow(const Context *ctx);
static U32 update(Dialog *dialog, U32 operation, const Context *ctx);

U64 displayOptions(Dialog *dialog, U32 operation, const Context *ctx) {
    if (operation == Startup)
        return startup(ctx);
    if (operation == PrepareWindow) {
        prepareWindow(ctx);
        return 0;
    }
    return update(dialog, operation, ctx);
}

static int valid(Size size) {
    return size.w >= 800 && size.w <= 7680 && size.h >= 600 && size.h <= 4320;
}

static U32 pack(Settings value) {
    return value.mode << 26 | value.size.h << 13 | value.size.w;
}

static int queueDisplay(Settings configuration, const Context *ctx) {
    void *window = GAME(getWindow, void *(*)(void))();
    if (!window)
        return 0;
    void *value = (void *)(U64)pack(configuration);
    GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->pendingKey, value);
    return GAME(getWindowData, void *(*)(void *, const char *))(window, ctx->pendingKey) == value;
}

static int displayFailed(const Context *ctx) {
    void *window = GAME(getWindow, void *(*)(void))();
    return window && GAME(setWindowData, void *(*)(void *, const char *, void *))(window, ctx->failedKey, 0);
}

static Settings load(const Context *ctx) {
    U32 packed = (U32)GAME(get, int (*)(const char *, int))(ctx->key, 0);
    Settings result = { packed >> 26, { packed & 8191, (packed >> 13) & 8191 } };
    if ((result.mode != Windowed && result.mode != Fullscreen && result.mode != Borderless) ||
        (result.mode != Borderless && !valid(result.size)))
        return (Settings){0, {0, 0}};
    if (result.mode == Borderless)
        result.size = (Size){0, 0};
    return result;
}

static U64 startup(const Context *ctx) {
    const Settings settings = load(ctx);
    if (!settings.mode)
        return 0;
    *GAME(windowed, Byte *) = settings.mode == Windowed;
    return (U64)settings.size.h << 32 | settings.size.w;
}

static void prepareWindow(const Context *ctx) {
    const char *override = GAME(getenv, const char *(*)(const char *))(ctx->resolutionVar);
    if (override && *override)
        return;
    const Settings settings = load(ctx);
    if (settings.mode == Fullscreen || settings.mode == Borderless)
        GAME(hint, Byte (*)(const char *, const char *, int))(ctx->spacesHint,
            settings.mode == Borderless ? ctx->disableSpaces : ctx->enableSpaces, 2);
}

static Object *child(Object *root, U32 id) {
    return METHOD(root, 0x118, Child)(root, id);
}

static void visible(Object *window, int show) {
    if (window)
        METHOD(window, 0x220, Flag)(window, 1, show != 0);
}

static Object *combo(Object *root, U32 id) {
    Object *result = 0;
    if (!METHOD(root, 0x128, Query)(root, id, ComboID, &result))
        return 0;
    return result;
}

static void displayResolution(Object *root, int borderless, int error) {
    visible(child(root, ResolutionID), !borderless);
    visible(child(root, ResolutionFrameID), !borderless);
    visible(child(root, DesktopID), borderless);
    visible(child(root, RestartID), 0);
    visible(child(root, ErrorID), error);
}

static int same(Size a, Size b) {
    return a.w == b.w && a.h == b.h;
}

static Size current(const Context *ctx) {
    Size size = {0, 0};
    int depth = 0;
    GAME(currentMode, void (*)(U32 *, U32 *, int *))(&size.w, &size.h, &depth);
    return valid(size) ? size : (Size){1024, 768};
}

static Settings initial(const Context *ctx, int defaults) {
    if (!defaults) {
        Settings saved = load(ctx);
        if (saved.mode) {
            if (saved.mode == Borderless)
                saved.size = current(ctx);
            return saved;
        }
    }
    int windowed = !defaults && GAME(inWindow, Byte (*)(void))();
    return (Settings){windowed ? Windowed : Fullscreen, current(ctx)};
}

static char *decimal(char *out, U32 n) {
    char digits[5];
    int count = 0;
    do {
        digits[count++] = '0' + n % 10;
        n /= 10;
    } while (n);
    while (count)
        *out++ = digits[--count];
    return out;
}

static U32 number(const char **p) {
    U32 n = 0;
    while (**p >= '0' && **p <= '9' && n < 10000)
        n = n * 10 + *(*p)++ - '0';
    return n;
}

static Size selectedSize(Object *resolution, const Context *ctx) {
    int index = METHOD(resolution, 0xe8, Selection)(resolution);
    if (index < 0)
        return (Size){0, 0};
    String label;
    GAME(emptyString, StringOp)(&label);
    Size size = {0, 0};
    if (METHOD(resolution, 0xf8, GetString)(resolution, index, &label)) {
        const char *p = GAME(toChar, Text)(&label);
        size.w = number(&p);
        if (*p++ == 'x') {
            size.h = number(&p);
            if (*p)
                size.h = 0;
        }
    }
    GAME(destroyString, StringOp)(&label);
    return size;
}

static int insert(Object *combo, Size size, int index, const Context *ctx) {
    char raw[12];
    char *next = decimal(raw, size.w);
    *next++ = 'x';
    *decimal(next, size.h) = 0;
    String label;
    GAME(string, TextInit)(&label, raw);
    int ok = METHOD(combo, 0xd8, Insert)(combo, &label, index);
    GAME(destroyString, StringOp)(&label);
    return ok;
}

static void addSize(Size *sizes, int *count, Size size) {
    if (!valid(size))
        return;
    for (int index = 0; index < *count; ++index)
        if (same(sizes[index], size))
            return;
    sizes[(*count)++] = size;
}

static void addDisplayMode(Size *sizes, int *count, const void *mode, const Context *ctx) {
    unsigned long w = GAME(pixelWidth, unsigned long (*)(const void *))(mode);
    unsigned long h = GAME(pixelHeight, unsigned long (*)(const void *))(mode);
    if (w <= 7680 && h <= 4320)
        addSize(sizes, count, (Size){(U32)w, (U32)h});
}

static int populateSizes(Object *combo, Size wanted, const Context *ctx) {
    METHOD(combo, 0xd0, Clear)(combo);
    Size sizes[128];
    int count = 1;
    sizes[0] = wanted;
    U32 display = GAME(displayID, U32 (*)(void))();
    if (!display)
        display = GAME(mainDisplayID, U32 (*)(void))();
    // The current desktop mode can be absent from the enumerated modes.
    const void *desktop = GAME(copyDesktopMode, const void *(*)(U32))(display);
    if (desktop) {
        addDisplayMode(sizes, &count, desktop, ctx);
        GAME(release, void (*)(const void *))(desktop);
    }
    int n = GAME(numModes, int (*)(void))();
    for (int i = 0; i < n && count < 128; ++i) {
        Size size = {0, 0};
        int depth = 0;
        GAME(getMode, void (*)(int, U32 *, U32 *, int *))(i, &size.w, &size.h, &depth);
        if (depth == 32)
            addSize(sizes, &count, size);
    }
    const void *available = GAME(copyDisplayModes, const void *(*)(U32, const void *))(display, 0);
    if (available) {
        long availableCount = GAME(arrayCount, long (*)(const void *))(available);
        for (long index = 0; index < availableCount && count < 128; ++index) {
            const void *mode = GAME(arrayValue, const void *(*)(const void *, long))(available, index);
            addDisplayMode(sizes, &count, mode, ctx);
        }
        GAME(release, void (*)(const void *))(available);
    }
    for (int i = 0; i < count; ++i)
        if (!insert(combo, sizes[i], i, ctx))
            return 0;
    return METHOD(combo, 0xe0, Select)(combo, 0, 0);
}

static int populateModes(Object *combo, U32 chosen, const Context *ctx) {
    METHOD(combo, 0xd0, Clear)(combo);
    for (U32 i = 0; i < 3; ++i) {
        String label;
        GAME(localizedString, LocaleInit)(&label, 0xe74a40da, 0xe74a44e4+i, 0);
        int ok = METHOD(combo, 0xd8, Insert)(combo, &label, i);
        GAME(destroyLocalized, StringOp)(&label);
        if (!ok)
            return 0;
    }
    return METHOD(combo, 0xe0, Select)(combo, chosen-1, 0);
}

static U32 update(Dialog *dialog, U32 operation, const Context *ctx) {
    Object *root = METHOD(dialog->host, 0x18, Window)(dialog->host);
    if (!root)
        return 0;
    Object *mode = combo(root, ModeID);
    if (!mode)
        return 0;
    Object *resolution = combo(root, ResolutionID);
    U32 result = 0;
    if (resolution) {
        if (operation == Open || operation == Defaults) {
            Settings settings = initial(ctx, operation == Defaults);
            if (populateSizes(resolution, settings.size, ctx) && populateModes(mode, settings.mode, ctx)) {
                for (U32 index = 0; index < 4; ++index) {
                    visible(child(root, 0x6a57da5a+index), 0);
                    visible(child(root, 0xe74a4410+index), 0);
                }
                Object *modeWindow = child(root, ModeID);
                METHOD(modeWindow, 0x2b0, Target)(modeWindow, root);
                visible(modeWindow, 1);
                visible(child(root, ModeFrameID), 1);
                visible(child(root, ModeTitleID), 1);
                displayResolution(root, settings.mode == Borderless, operation == Open && displayFailed(ctx));
            }
        } else {
            int index = METHOD(mode, 0xe8, Selection)(mode);
            if (index >= 0 && index < 3) {
                const U32 selected = index+1;
                if (operation == ModeChanged) {
                    displayResolution(root, selected == Borderless, 0);
                } else {
                    Settings configuration = { selected, {0, 0} };
                    if (selected != Borderless)
                        configuration.size = selectedSize(resolution, ctx);
                    if (selected == Borderless || valid(configuration.size)) {
                        if (!queueDisplay(configuration, ctx)) {
                            displayResolution(root, selected == Borderless, 1);
                            result = 2;
                        }
                    }
                }
            }
        }
        METHOD(resolution, 0x10, Release)(resolution);
    }
    METHOD(mode, 0x10, Release)(mode);
    return result;
}
