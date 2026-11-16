// Aspyr 185949. Compile as freestanding x86_64-linux-gnu and extract .text.
typedef unsigned long U64;
typedef unsigned U32;

U64 fontLayoutBounds(const void *face) {
    const char *record = face;
    const int *bounds = (const int *)(record + 0x58);
    const unsigned short units = *(const unsigned short *)(record + 0x68);
    const short ascender = *(const short *)(record + 0x6a);
    int top = bounds[3], bottom = bounds[1];

    // Aspyr measures lines using the face-wide bounding box. A few added
    // glyphs extend it well beyond the height of the original UI fonts.
    // Match the replacement face metrics and keep the original line box.
    if (units == 1000 && ascender == 935 &&
        ((top == 1152 && bottom == -354) || (top == 1092 && bottom == -352))) {
        top = 921; bottom = -250; // Arta, regular and italic.
    } else if (units == 1000 && ascender == 1018 && top == 1009 && bottom == -273) {
        top = 909; bottom = -236; // Stoclet ITC TT, bold.
    } else if (units == 2048 && ascender == 1854 && top == 2136 && bottom == -797) {
        top = 1878; bottom = -431; // Arial MT, all styles.
    } else if (units == 2048 && ascender == 1705 &&
               ((top == 2009 && bottom == -615) || (top == 2024 && bottom == -771))) {
        top = 1864; bottom = -621; // Courier LD, all styles.
    }
    return (U32)top | (U64)(U32)bottom << 32;
}
