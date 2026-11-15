// Compile for i386 Linux with -Os -ffreestanding -fno-pic -fno-stack-protector.
// The .text payload uses the stock EXE's thiscall ABI and resolved Win32 APIs.
typedef unsigned U32;
typedef unsigned short Wide;
typedef unsigned char Byte;
#define TC __attribute__((thiscall))
#define WINAPI __attribute__((stdcall))
#define METHOD(object, slot, type) ((type)(object)->methods[(slot) / 4])

typedef struct { void **methods; } Object;
typedef struct { Object window; Object *host; } Dialog;
typedef struct { void **methods; const char *begin, *end, *limit; U32 refs; } String;
typedef struct { String string; U32 resource[3]; } TextResource;
typedef struct { U32 width, height; } Size;
typedef struct {
    Wide name[32];
    unsigned short spec, driver, size, extra;
    Byte fields[96];
    U32 depth, width, height;
    Byte remainder[40];
} DisplayMode;
typedef struct { U32 size; int bounds[8]; U32 flags; Wide device[32]; } MonitorInfo;
enum { PathCapacity = 260, Combo = 0xe74a4401, Borderless = 0xe74a4402, SaveError = 0xe74a4403,
       ModeCombo = 0xe74a4404, ModeTitle = 0xe74a4405, ResolutionFrame = 0xe74a4406,
       ModeFrame = 0xe74a4407, ComboInterface = 0x412ce496 };
enum { Windowed, FullScreen, BorderlessFullScreen };

typedef void *(WINAPI *Module)(const char *);
typedef void *(WINAPI *Resolve)(void *, const char *);
typedef struct {
    Module *module;
    Resolve *resolve;
    void **stringMethods;
    void (TC *loadText)(TextResource *, U32, U32, U32);
    void (TC *closeText)(TextResource *);
    U32 localeGroup;
    U32 firstModeText;
    char names[];
} Data;

typedef struct {
    U32 (WINAPI *modulePath)(void *, Wide *, U32);
    U32 (WINAPI *readString)(const Wide *, const Wide *, const Wide *, Wide *, U32, const Wide *);
    U32 (WINAPI *readInt)(const Wide *, const Wide *, int, const Wide *);
    int (WINAPI *writeString)(const Wide *, const Wide *, const Wide *, const Wide *);
    U32 (WINAPI *attributes)(const Wide *);
    U32 (WINAPI *tempFile)(const Wide *, const Wide *, U32, Wide *);
    int (WINAPI *copy)(const Wide *, const Wide *, int);
    int (WINAPI *move)(const Wide *, const Wide *, U32);
    int (WINAPI *remove)(const Wide *);
    void *(WINAPI *activeWindow)(void);
    void *(WINAPI *monitorForWindow)(void *, U32);
    int (WINAPI *monitorInfo)(void *, MonitorInfo *);
    int (WINAPI *enumMode)(const Wide *, U32, DisplayMode *);
} Win32;
typedef struct {
    const Wide *filename, *section, *mode, *width, *height, *borderless, *borderlessFullscreen, *prefix;
    const Wide *windowed, *fullscreen;
} Keys;

typedef Object *(TC *Window)(Object *);
typedef Object *(TC *Child)(Object *, U32);
typedef Byte (TC *Query)(Object *, U32, U32, Object **);
typedef void (TC *Release)(Object *);
typedef void (TC *SetFlag)(Object *, U32, Byte);
typedef Byte (TC *Visible)(Object *);
typedef void (TC *SetMessageTarget)(Object *, Object *);
typedef void (TC *ClearItems)(Object *);
typedef Byte (TC *InsertItem)(Object *, const String *, int);
typedef Byte (TC *SelectItem)(Object *, int, Byte);
typedef int (TC *SelectedItem)(Object *);
typedef Byte (TC *ItemReference)(Object *, int, const String **);

static Object *child(Object *root, U32 id) {
    return METHOD(root, 0x8c, Child)(root, id);
}

static void flag(Object *window, U32 mask, Byte value) {
    if (window)
        METHOD(window, 0x110, SetFlag)(window, mask, value);
}

static int valid(Size size) {
    return size.width >= 800 && size.height >= 600 && size.width <= 99999 && size.height <= 99999;
}

static int same(Size first, Size second) {
    return first.width == second.width && first.height == second.height;
}

static int less(Size first, Size second) {
    return first.width < second.width || (first.width == second.width && first.height < second.height);
}

static char *decimal(char *out, U32 number) {
    char reversed[10];
    U32 count = 0;
    do {
        reversed[count++] = '0' + number % 10;
        number /= 10;
    } while (number);
    while (count)
        *out++ = reversed[--count];
    *out = 0;
    return out;
}

static U32 integer(const char **p, const char *end) {
    U32 value = 0;
    while (*p < end && **p >= '0' && **p <= '9' && value < 100000)
        value = value * 10 + *(*p)++ - '0';
    return value;
}

static Size itemSize(Object *window, int index) {
    const String *label = 0;
    if (index < 0 || !METHOD(window, 0x264, ItemReference)(window, index, &label))
        return (Size){0, 0};
    const char *p = label->begin;
    Size size = {integer(&p, label->end), 0};
    if (p < label->end && *p++ == 'x')
        size.height = integer(&p, label->end);
    return p == label->end && valid(size) ? size : (Size){0, 0};
}

static int insertSize(Object *window, Object *combo, Size size, void **stringMethods) {
    int index = 0;
    for (;;) {
        Size item = itemSize(window, index);
        if (same(item, size))
            return index;
        if (less(item, size))
            break;
        ++index;
    }

    char label[12];
    char *next = decimal(label, size.width);
    *next++ = 'x';
    next = decimal(next, size.height);
    const String text = {stringMethods, label, next, label + sizeof(label), 0};
    return METHOD(combo, 0x6c, InsertItem)(combo, &text, index) ? index : -1;
}

static Wide lowercase(Wide c) {
    return c >= 'A' && c <= 'Z' ? c + ('a' - 'A') : c;
}

static int equalMode(const Wide *first, const Wide *second) {
    while (*first && lowercase(*first) == lowercase(*second))
        ++first, ++second;
    return *first == *second;
}

static int loadAPI(const Data *data, Win32 *api, Keys *keys, void **plugin) {
    const char *name = data->names;
    void *kernel = (*data->module)(name);
    while (*name++) {}
    void *user = (*data->module)(name);
    while (*name++) {}
    *plugin = (*data->module)(name);
    if (!kernel || !user || !*plugin)
        return 0;
    while (*name++) {}

    // The data stores the export names in this order, followed by UTF-16 keys.
#define LOAD(field, library) \
    api->field = (__typeof__(api->field))(*data->resolve)(library, name); \
    if (!api->field) return 0; \
    while (*name++) {}
    LOAD(modulePath, kernel)
    LOAD(readString, kernel)
    LOAD(readInt, kernel)
    LOAD(writeString, kernel)
    LOAD(attributes, kernel)
    LOAD(tempFile, kernel)
    LOAD(copy, kernel)
    LOAD(move, kernel)
    LOAD(remove, kernel)
    LOAD(activeWindow, user)
    LOAD(monitorForWindow, user)
    LOAD(monitorInfo, user)
    LOAD(enumMode, user)
#undef LOAD
    const Wide *key = (const Wide *)(((U32)name + 1) & ~1U);
#define KEY(field) keys->field = key; while (*key++) {}
    KEY(filename)
    KEY(section)
    KEY(mode)
    KEY(width)
    KEY(height)
    KEY(borderless)
    KEY(borderlessFullscreen)
    KEY(windowed)
    KEY(fullscreen)
    keys->prefix = key;
#undef KEY
    return 1;
}

static int configPath(const Win32 *api, void *plugin, const Wide *filename,
                      Wide *path, U32 *directoryLength) {
    U32 length = api->modulePath(plugin, path, PathCapacity);
    if (!length || length >= PathCapacity)
        return 0;
    while (length && path[length - 1] != '\\' && path[length - 1] != '/')
        --length;
    if (!length)
        return 0;
    *directoryLength = length;
    while (*filename && length + 1 < PathCapacity)
        path[length++] = *filename++;
    path[length] = 0;
    return !*filename;
}

static const Wide *currentDisplay(const Win32 *api, MonitorInfo *info) {
    info->size = sizeof(*info);
    void *monitor = api->monitorForWindow(api->activeWindow(), 1);
    return monitor && api->monitorInfo(monitor, info) ? info->device : 0;
}

static Size displaySize(const Win32 *api, const Wide *device, U32 index, U32 *depth) {
    DisplayMode mode;
    Byte *bytes = (Byte *)&mode;
    for (U32 i = 0; i < sizeof(mode); ++i)
        bytes[i] = 0;
    mode.size = sizeof(mode);
    if (!api->enumMode(device, index, &mode))
        return (Size){0, 0};
    *depth = mode.depth;
    return (Size){mode.width, mode.height};
}

static void wideDecimal(Wide *out, U32 value) {
    char digits[11];
    decimal(digits, value);
    const char *p = digits;
    do {
        *out++ = *p;
    } while (*p++);
}

static int saveDisplay(const Win32 *api, const Keys *keys, const Wide *path,
                       U32 directoryLength, Size size, const Wide *mode) {
    const U32 attributes = api->attributes(path);
    if (attributes == (U32)-1 || (attributes & 0x11)) // Read-only or directory.
        return 0;
    Wide directory[PathCapacity], temporary[PathCapacity];
    for (U32 index = 0; index < directoryLength; ++index)
        directory[index] = path[index];
    directory[directoryLength] = 0;
    if (!api->tempFile(directory, keys->prefix, 0, temporary))
        return 0;

    Wide width[6], height[6];
    wideDecimal(width, size.width);
    wideDecimal(height, size.height);
    int saved = api->copy(path, temporary, 0)
        && (!mode || api->writeString(keys->section, keys->mode, mode, temporary))
        && (!valid(size) || (api->writeString(keys->section, keys->width, width, temporary)
                           && api->writeString(keys->section, keys->height, height, temporary)))
        && api->move(temporary, path, 0x9); // REPLACE_EXISTING | WRITE_THROUGH
    if (!saved)
        api->remove(temporary);
    return saved;
}

static void showResolution(Object *root, Object *window, int borderless) {
    flag(window, 1, !borderless);
    flag(child(root, ResolutionFrame), 1, !borderless);
    flag(child(root, Borderless), 1, borderless);
}

static void showPicker(Object *root, Object *window, Object *mode, int show, int borderless) {
    for (U32 index = 0; index < 4; ++index) {
        flag(child(root, 0x6a57da5a + index), 1, !show);
        flag(child(root, 0xe74a4410 + index), 1, !show);
    }
    flag(window, 1, show && !borderless);
    flag(child(root, ResolutionFrame), 1, show && !borderless);
    flag(child(root, Borderless), 1, show && borderless);
    flag(mode, 1, show);
    flag(child(root, ModeFrame), 1, show);
    flag(child(root, ModeTitle), 1, show);
    flag(child(root, SaveError), 1, 0);
}

static int windowModeIndex(const Keys *keys, const Wide *mode) {
    if (equalMode(mode, keys->windowed))
        return Windowed;
    if (equalMode(mode, keys->borderless) || equalMode(mode, keys->borderlessFullscreen))
        return BorderlessFullScreen;
    return FullScreen;
}

static int populateModes(Object *combo, const Data *data, int selected) {
    METHOD(combo, 0x68, ClearItems)(combo);
    for (U32 index = 0; index < 3; ++index) {
        TextResource title;
        data->loadText(&title, data->localeGroup, data->firstModeText + index, 0);
        const int inserted = title.string.begin != title.string.end
            && METHOD(combo, 0x6c, InsertItem)(combo, &title.string, index);
        data->closeText(&title);
        if (!inserted)
            return 0;
    }
    return METHOD(combo, 0x70, SelectItem)(combo, selected, 0);
}

// Return 1 if a saved change needs the stock restart notice, 2 on a write
// failure that must keep the settings open, and 0 for other operations.
U32 graphicsResolution(Dialog *dialog, U32 operation, const Data *data) {
    Object *root = METHOD(dialog->host, 0x0c, Window)(dialog->host);
    Object *window = child(root, Combo);
    if (!window)
        return 0;
    Object *modeWindow = child(root, ModeCombo);
    if (!modeWindow)
        return 0;

    if (operation == 3) {
        Object *modeCombo = 0;
        if (METHOD(modeWindow, 0x11c, Visible)(modeWindow)
            && METHOD(root, 0x94, Query)(root, ModeCombo, ComboInterface, &modeCombo) && modeCombo) {
            showResolution(root, window, METHOD(modeCombo, 0x78, SelectedItem)(modeCombo) == BorderlessFullScreen);
            METHOD(modeCombo, 0x08, Release)(modeCombo);
        }
        return 0;
    }

    Win32 api;
    Keys keys;
    void *plugin;
    Wide path[PathCapacity], windowMode[32], empty = 0;
    U32 directoryLength;
    if (!loadAPI(data, &api, &keys, &plugin) ||
        !configPath(&api, plugin, keys.filename, path, &directoryLength))
        return 0;
    if (!api.readString(keys.section, keys.mode, &empty, windowMode, 32, path))
        return 0;
    const int configuredMode = windowModeIndex(&keys, windowMode);
    Object *combo = 0;
    Object *modeCombo = 0;
    if (!METHOD(root, 0x94, Query)(root, Combo, ComboInterface, &combo) || !combo)
        return 0;
    if (!METHOD(root, 0x94, Query)(root, ModeCombo, ComboInterface, &modeCombo) || !modeCombo) {
        METHOD(combo, 0x08, Release)(combo);
        return 0;
    }

    U32 depth;
    MonitorInfo monitor;
    const Wide *device = currentDisplay(&api, &monitor);
    const Size desktop = displaySize(&api, device, (U32)-1, &depth);
    const Size configured = {api.readInt(keys.section, keys.width, desktop.width, path),
                             api.readInt(keys.section, keys.height, desktop.height, path)};
    U32 result = 0;
    if (operation == 2) {
        if (METHOD(modeWindow, 0x11c, Visible)(modeWindow)) {
            int selectedMode = METHOD(modeCombo, 0x78, SelectedItem)(modeCombo);
            if (selectedMode < Windowed || selectedMode > BorderlessFullScreen)
                selectedMode = configuredMode;
            const Wide *mode = 0;
            if (selectedMode != configuredMode)
                mode = selectedMode == Windowed ? keys.windowed :
                       selectedMode == FullScreen ? keys.fullscreen : keys.borderlessFullscreen;
            Size selection = selectedMode == BorderlessFullScreen ? (Size){0, 0} :
                itemSize(window, METHOD(combo, 0x78, SelectedItem)(combo));
            if (same(selection, configured))
                selection = (Size){0, 0};
            if (mode || valid(selection)) {
                result = saveDisplay(&api, &keys, path, directoryLength, selection, mode) ? 1 : 2;
                flag(child(root, SaveError), 1, result == 2);
            }
        }
    } else {
        METHOD(combo, 0x68, ClearItems)(combo);
        for (U32 index = 0; ; ++index) {
            Size mode = displaySize(&api, device, index, &depth);
            if (!mode.width)
                break;
            if (depth == 32 && valid(mode))
                insertSize(window, combo, mode, data->stringMethods);
        }
        Size initial = operation == 0 && valid(configured) ? configured : desktop;
        if (!valid(initial))
            initial = itemSize(window, 0);
        int index = valid(initial) ? insertSize(window, combo, initial, data->stringMethods) : -1;
        int selectedMode = operation == 0 ? configuredMode : FullScreen;
        int show = index >= 0 && METHOD(combo, 0x70, SelectItem)(combo, index, 0)
            && populateModes(modeCombo, data, selectedMode);
        if (show)
            METHOD(modeWindow, 0x158, SetMessageTarget)(modeWindow, root);
        showPicker(root, window, modeWindow, show, selectedMode == BorderlessFullScreen);
    }
    METHOD(modeCombo, 0x08, Release)(modeCombo);
    METHOD(combo, 0x08, Release)(combo);
    return result;
}
