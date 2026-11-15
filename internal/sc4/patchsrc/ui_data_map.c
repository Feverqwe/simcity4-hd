// clang -target i386-linux-gnu -march=i386 -Os -ffreestanding -fno-pic -fno-stack-protector -c ui_data_map.c -o /tmp/sc4-data-map.o
// llvm-objcopy -O binary -j .text /tmp/sc4-data-map.o ui_data_map.i386
// llvm-objcopy -O binary -j .text.terrain /tmp/sc4-data-map.o ui_map_terrain.i386
// llvm-objcopy -O binary -j .text.tile /tmp/sc4-data-map.o ui_map_terrain_tile.i386
// Each section has a relocation-free entry at offset zero.
typedef unsigned int Color;
typedef unsigned char Byte;
#define THISCALL __attribute__((thiscall))

typedef struct { void **methods; } Bitmap;
typedef struct { const Byte *first, *end, *capacity; } GridColumn;
typedef struct { const GridColumn *first, *end, *capacity; } Grid;
typedef Byte (THISCALL *Lock)(Bitmap *, unsigned);
typedef int (THISCALL *Measure)(Bitmap *);
typedef Color *(THISCALL *Pixels)(Bitmap *);
typedef void (*Raster)(const Grid *, int, int, Bitmap *, int, int, const Color *, unsigned);

typedef struct {
    unsigned alpha, red, green, blue;
} WeightedColor;

static inline __attribute__((always_inline)) void addColor(WeightedColor *sum, Color ink, unsigned weight) {
    sum->alpha += (ink >> 24) * weight;
    sum->red += ((ink >> 16) & 255) * weight;
    sum->green += ((ink >> 8) & 255) * weight;
    sum->blue += (ink & 255) * weight;
}

typedef struct { int first, second, fraction; } Sample;

static inline Sample sample(int pixel, int step, int count) {
    // Source colors are centered in the blocks drawn by the original rasterizer.
    int position = 2 * pixel + 1 - step;
    if (position <= 0)
        return (Sample){0, 0, 0};
    const int first = position / (2 * step);
    if (first >= count - 1)
        return (Sample){count - 1, count - 1, 0};
    return (Sample){first, first + 1, position % (2 * step)};
}

static int render(const Grid *grid, int columns, int rows, Bitmap *bitmap,
                  int width, int height, const Color *palette, unsigned rotation, unsigned mode);

// The same byte-grid rasterizer also draws discrete utility and zone states.
void drawDataMap(const Grid *grid, int columns, int rows, Bitmap *bitmap,
                 int width, int height, const Color *palette, unsigned rotation,
                 unsigned mode, Raster stock) {
    if (!render(grid, columns, rows, bitmap, width, height, palette, rotation, mode))
        stock(grid, columns, rows, bitmap, width, height, palette, rotation);
}

static int render(const Grid *grid, int columns, int rows, Bitmap *bitmap,
                  int width, int height, const Color *palette, unsigned rotation, unsigned mode) {
    enum { power = 7, water = 10, zones = 30 };
    if (mode == power || mode == water || mode == zones || rotation > 3 ||
        !bitmap || !palette || !grid || !grid->first ||
        width < 512 || width > 2048 || width != height ||
        columns <= 0 || columns > width || rows <= 0 || rows > height ||
        grid->end - grid->first < columns || width % columns || height % rows)
        return 0;
    const int stepX = width / columns, stepY = height / rows;
    if ((stepX == 1 && stepY == 1) || (stepX & (stepX - 1)) || (stepY & (stepY - 1)) ||
        stepX > 128 || stepY > 128 || ((rotation & 1) && stepX != stepY))
        return 0;
    for (int x = 0; x < columns; ++x) {
        const GridColumn *column = grid->first + x;
        if (!column->first || column->end - column->first < rows)
            return 0;
    }

    const unsigned lock = 0x8040;
    if (!((Lock)bitmap->methods[0x18 / 4])(bitmap, lock))
        return 1;
    Color *pixels = ((Pixels)bitmap->methods[0x88 / 4])(bitmap);
    const int strideBytes = ((Measure)bitmap->methods[0x8c / 4])(bitmap);
    const int valid = pixels && strideBytes >= width * 4 && strideBytes <= 32768 && !(strideBytes & 3);
    if (valid) {
        const int stride = strideBytes / 4;
        int dx = 1, dy = stride;
        // Traverse the normal game rotation directly in the destination bitmap.
        if (rotation == 1) {
            pixels += (width - 1) * stride;
            dx = -stride;
            dy = 1;
        } else if (rotation == 2) {
            pixels += (height - 1) * stride + width - 1;
            dx = -1;
            dy = -stride;
        } else if (rotation == 3) {
            pixels += height - 1;
            dx = stride;
            dy = -1;
        }

        const unsigned normalization = 4 * stepX * stepY;
        for (int y = 0; y < height; ++y) {
            const Sample sy = sample(y, stepY, rows);
            Color *row = pixels + y * dy;
            const unsigned wy0 = 2 * stepY - sy.fraction;
            for (int x = 0; x < width; ++x) {
                const Sample sx = sample(x, stepX, columns);
                const Byte *left = grid->first[sx.first].first;
                const Byte *right = grid->first[sx.second].first;
                const Color topLeft = palette[left[sy.first]];
                const Color topRight = palette[right[sy.first]];
                const Color bottomLeft = palette[left[sy.second]];
                const Color bottomRight = palette[right[sy.second]];
                if (topLeft == topRight && topLeft == bottomLeft && topLeft == bottomRight) {
                    row[x * dx] = topLeft;
                    continue;
                }
                const unsigned wx0 = 2 * stepX - sx.fraction;
                WeightedColor sum = {0, 0, 0, 0};
                addColor(&sum, topLeft, wx0 * wy0);
                addColor(&sum, topRight, sx.fraction * wy0);
                addColor(&sum, bottomLeft, wx0 * sy.fraction);
                addColor(&sum, bottomRight, sx.fraction * sy.fraction);
                // The stock map compositor expects premultiplied RGB.
                const unsigned round = normalization / 2;
                row[x * dx] = ((sum.alpha + round) / normalization << 24) |
                              ((sum.red + round) / normalization << 16) |
                              ((sum.green + round) / normalization << 8) |
                              ((sum.blue + round) / normalization);
            }
        }
    }
    ((Lock)bitmap->methods[0x1c / 4])(bitmap, lock);
    return valid;
}

typedef void *(*Allocate)(unsigned);
typedef void (*Free)(void *);
typedef void (*PaintTerrain)(Bitmap *, const Color *, int, int, unsigned);
typedef void (*PaintOverlay)(Bitmap *, const Color *, int, int, Bitmap *, unsigned);

static inline __attribute__((always_inline)) Color mixTerrain(Color a, Color b, Color c, Color d,
                                                               unsigned fx, unsigned fy, unsigned scale) {
    const unsigned weightA = (scale - fx) * (scale - fy);
    const unsigned weightB = fx * (scale - fy);
    const unsigned weightC = (scale - fx) * fy;
    const unsigned weightD = fx * fy;
    const unsigned shift = scale == 4 ? 4 : scale == 8 ? 6 : 8;
    const unsigned round = (scale * scale) / 2;
    const unsigned rb = 0x00ff00ff, g = 0x0000ff00;
    const unsigned redBlue = (a & rb) * weightA + (b & rb) * weightB +
                             (c & rb) * weightC + (d & rb) * weightD;
    const unsigned green = (a & g) * weightA + (b & g) * weightB +
                           (c & g) * weightC + (d & g) * weightD;
    return (((redBlue + (round << 16) + round) >> shift) & rb) |
           (((green + (round << 8)) >> shift) & g);
}

__attribute__((section(".text.terrain")))
void drawMapTerrain(Bitmap *destination, const Color *terrain, int stride, int size,
                    Bitmap *overlay, unsigned rotation, int zoomShift,
                    void *paint, Allocate allocate, Free release) {
    Color *filtered = 0;
    // At these zoom levels the stock terrain raster repeats each tile.
    // Keep the cached raster intact, including partial updates and network markers.
    if (terrain && size >= 128 && size <= 1024 && stride == size && !(size & 3) &&
        (zoomShift <= -1 && zoomShift >= -3)) {
        filtered = allocate(size * size * sizeof(Color));
        if (filtered) {
            const int step = 1 << -zoomShift;
            const int count = size / step;
            const Color marker = 0xffcfcfc5;
            for (int y = 0; y < size; ++y) {
                const Sample sy = sample(y, step, count);
                const Color *top = terrain + (sy.first * step + step / 2) * stride;
                const Color *bottom = terrain + (sy.second * step + step / 2) * stride;
                const Color *source = terrain + y * stride;
                Color *row = filtered + y * stride;
                for (int x = 0; x < size; ++x) {
                    const Sample sx = sample(x, step, count);
                    const int left = sx.first * step + step / 2;
                    const int right = sx.second * step + step / 2;
                    row[x] = source[x] == marker ? marker :
                             mixTerrain(top[left], top[right], bottom[left], bottom[right],
                                        sx.fraction, sy.fraction, 2 * step);
                }
            }
        }
    }
    const Color *base = filtered ? filtered : terrain;
    if (overlay)
        ((PaintOverlay)paint)(destination, base, stride, size, overlay, rotation);
    else
        ((PaintTerrain)paint)(destination, base, stride, size, rotation);
    if (filtered)
        release(filtered);
}

// The original chunk dispatcher only has writers for up to 4 pixels per tile.
__attribute__((section(".text.tile")))
void expandTerrainChunk(Color *destination, int destinationPitch, const Color *tiles,
                        int tilesPitch, int width, int height) {
    if (width != 128 || height != 128 || destinationPitch < width * 4 || tilesPitch != 64)
        return;
    const int stride = destinationPitch / 4;
    for (int y = 0; y < height; ++y) {
        const Color *source = tiles + (y / 8) * (tilesPitch / 4);
        for (int x = 0; x < width; ++x)
            destination[x] = source[x / 8];
        destination += stride;
    }
}
