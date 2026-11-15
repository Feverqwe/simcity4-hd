// clang -target i386-linux-gnu -march=i386 -Os -ffreestanding -fno-pic -fno-stack-protector -fno-math-errno -c ui_data_map.c -o /tmp/sc4-data-map.o
// llvm-objcopy -O binary -j .text.tile /tmp/sc4-data-map.o ui_map_terrain_tile.i386
// llvm-objcopy -O binary -j .text.sample /tmp/sc4-data-map.o ui_map_terrain_sample.i386
typedef unsigned int Color;

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

typedef unsigned char Byte;
#define THISCALL __attribute__((thiscall))

typedef struct { float x, y, z; } Vector;
typedef struct { void **methods; } Terrain;
typedef float (THISCALL *Measure)(Terrain *);
typedef float (THISCALL *Sample)(Terrain *, float x, float z);
typedef void (THISCALL *Normal)(Terrain *, float x, float z, Vector *);
typedef Byte (THISCALL *HasWater)(Terrain *, unsigned x, unsigned z);

typedef struct {
    Byte beforeZ[0x14];
    int z;
    Byte beforeTerrain[0x18];
    Terrain *terrain;
    int x;
    Byte beforeLight[4];
    Vector light;
    Byte beforeSeaLevel[0x14];
    float seaLevel;
    Byte beforeColors[0x18];
    Color tiles[16 * 16];
} ChunkFrame;

typedef struct {
    Byte beforeWidth[0xe4];
    int width;
    Byte beforeLand[0x78];
    Color land[256];
    Color water[256];
} MiniMap;

static inline int level(float index, int maximum) {
    const volatile int bound = maximum;
    if (!(index > 0))
        return 0;
    if (index >= bound)
        return bound;
    return (int)index;
}

static inline unsigned channel(Color land, Color water, unsigned light, unsigned alpha) {
    const unsigned shaded = ((land & 255) * light + 128) >> 8;
    return (shaded * (256 - alpha) + (water & 255) * alpha + 128) >> 8;
}

enum WaterKind { dry, ocean, inland };

static enum WaterKind cellWater(Terrain *terrain, unsigned x, unsigned z) {
    if (!((HasWater)terrain->methods[0x88 / 4])(terrain, x, z))
        return dry;
    const HasWater isSea = (HasWater)terrain->methods[0x98 / 4];
    return isSea(terrain, x, z) || isSea(terrain, x + 1, z) ||
           isSea(terrain, x, z + 1) || isSea(terrain, x + 1, z + 1) ? ocean : inland;
}

static inline Color terrainColor(const MiniMap *map, const ChunkFrame *frame,
                                 float x, float z, enum WaterKind wet) {
    Terrain *terrain = frame->terrain;
    const float half = *(const float *)0xa84d2c;
    const float heightScale = *(const float *)0xab8370;
    const float slopeScale = *(const float *)0xab836c;
    const float ambient = *(const float *)0xb4e1b0;

    // Windows exposes the continuous altitude query after the vertex-index overload.
    const float altitude = ((Sample)terrain->methods[0x34 / 4])(terrain, x, z);
    const volatile int seaIndex = 64;
    const Color land = map->land[level((altitude - frame->seaLevel) * heightScale + seaIndex, 255)];
    Color water = map->water[0];
    if (wet != dry) {
        // Open water uses the sea plane. Inland water has its own modeled surface.
        const float surface = wet == ocean ? frame->seaLevel :
            ((Sample)terrain->methods[0x80 / 4])(terrain, x, z);
        water = map->water[level(surface - altitude + half, 255)];
    }
    const unsigned alpha = water >> 24;
    float sunlight = frame->light.y;
    if (altitude >= frame->seaLevel) {
        Vector n = {0, 1, 0};
        ((Normal)terrain->methods[0x6c / 4])(terrain, x, z, &n);
        n.y *= slopeScale;
        const float length = __builtin_sqrtf(n.x * n.x + n.y * n.y + n.z * n.z);
        if (length > 0)
            sunlight = (frame->light.x * n.x + frame->light.y * n.y + frame->light.z * n.z) / length;
    }
    const unsigned light = level(ambient + (sunlight > 0 ? 2 * sunlight : 0) + half, 256);
    return channel(land, water, light, alpha) |
           (channel(land >> 8, water >> 8, light, alpha) << 8) |
           (channel(land >> 16, water >> 16, light, alpha) << 16);
}

__attribute__((section(".text.sample")))
int renderTerrainChunk(Color *destination, int side, const MiniMap *map, const ChunkFrame *frame) {
    if (!destination || !frame->terrain || map->width != 512 ||
        (side != 32 && side != 64 && side != 128))
        return 0;
    const float cellWidth = ((Measure)frame->terrain->methods[0x18 / 4])(frame->terrain);
    const volatile int maxCellWidth = 64;
    if (!(cellWidth > 0 && cellWidth <= maxCellWidth))
        return 0;

    const float half = *(const float *)0xa84d2c;
    const int pixelsPerCell = side / 16;
    const float pixelWidth = cellWidth / pixelsPerCell;
    const float left = frame->x * cellWidth + pixelWidth * half;
    const float top = frame->z * cellWidth + pixelWidth * half;
    Byte water[16 * 16];
    for (int z = 0; z < 16; ++z)
        for (int x = 0; x < 16; ++x)
            water[z * 16 + x] = cellWater(frame->terrain, frame->x + x, frame->z + z);

    for (int y = 0; y < side; ++y) {
        const float z = top + y * pixelWidth;
        const int row = (y / pixelsPerCell) * 16;
        for (int x = 0; x < side; ++x) {
            const int index = row + x / pixelsPerCell;
            const Color tile = frame->tiles[index];
            // The stock tile painter tags categorical zoning overrides with alpha.
            destination[x] = tile >> 24 ? tile :
                terrainColor(map, frame, left + x * pixelWidth, z, water[index]);
        }
        destination += map->width;
    }
    return 1;
}
