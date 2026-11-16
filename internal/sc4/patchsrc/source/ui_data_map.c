// clang -target i386-linux-gnu -march=i386 -Os -ffreestanding -fno-pic -fno-stack-protector -fno-math-errno -c ui_data_map.c -o /tmp/sc4-data-map.o
// llvm-objcopy -O binary -j .text.tile /tmp/sc4-data-map.o ui_map_terrain_tile.i386
// llvm-objcopy -O binary -j .text.sample /tmp/sc4-data-map.o ui_map_terrain_sample.i386
// Aspyr: clang -target x86_64-linux-gnu -DSC4_ASPYR -Os -ffreestanding -fno-stack-protector -fno-math-errno -c ui_data_map.c -o /tmp/sc4-data-map-aspyr.o
// Link at zero with ld.lld --entry=paintAspyrTerrainChunk and SECTIONS { .text 0 : { *(.text.aspyr) *(.text*) *(.rodata*) } }.
// Extract the linked .text as ui_map_terrain_macho.x86_64.
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
#ifdef SC4_ASPYR
#define THISCALL
#else
#define THISCALL __attribute__((thiscall))
#endif

typedef struct { float x, y, z; } Vector;
typedef struct { void **methods; } Terrain;
typedef float (THISCALL *Measure)(Terrain *);
typedef float (THISCALL *Sample)(Terrain *, float x, float z);
typedef void (THISCALL *Normal)(Terrain *, float x, float z, Vector *);
typedef Byte (THISCALL *HasWater)(Terrain *, unsigned x, unsigned z);

#ifdef SC4_ASPYR
// cSC4WinMiniMap::UpdateTerrainAndZoneMap frame starting at RBP-0xc90.
typedef struct {
    int x;
    Byte beforeTerrain[4];
    Terrain *terrain;
    Byte beforeSeaLevel[8];
    float seaLevel;
    float lightX;
    Byte beforeZ[0x10];
    int z;
    Byte beforeLightZ[0x18];
    float lightZ;
    Byte beforeColors[0x10];
    Color tiles[16 * 16];
} ChunkFrame;

typedef struct {
    Byte beforeWidth[0x164];
    int width;
    Byte beforeZoom[0x24];
    int zoomShift;
    Byte beforePixels[0x18];
    Color *pixels;
    int stride;
    int height;
    Byte beforeLand[0x40];
    Color land[256];
    Color water[256];
} MiniMap;

enum { cellWidthSlot = 0x30, altitudeSlot = 0x60, normalSlot = 0xd8,
       waterSurfaceSlot = 0x100, cellHasWaterSlot = 0x110, isSeaSlot = 0x130 };
#else
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

enum { cellWidthSlot = 0x18, altitudeSlot = 0x34, normalSlot = 0x6c,
       waterSurfaceSlot = 0x80, cellHasWaterSlot = 0x88, isSeaSlot = 0x98 };
#endif

#define METHOD(terrain, offset, type) ((type)(terrain)->methods[(offset) / sizeof(void *)])

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
    if (!METHOD(terrain, cellHasWaterSlot, HasWater)(terrain, x, z))
        return dry;
    const HasWater isSea = METHOD(terrain, isSeaSlot, HasWater);
    return isSea(terrain, x, z) || isSea(terrain, x + 1, z) ||
           isSea(terrain, x, z + 1) || isSea(terrain, x + 1, z + 1) ? ocean : inland;
}

static inline Color terrainColor(const MiniMap *map, const ChunkFrame *frame,
                                 float x, float z, enum WaterKind wet) {
    Terrain *terrain = frame->terrain;
#ifdef SC4_ASPYR
    const float half = 0.5f;
    const float heightScale = 0.2f;
    const float slopeScale = 0.4f;
    const float ambient = 127.75f;
    // Aspyr's frame already scales its light direction by the brightness.
    const Vector aspyrLight = {frame->lightX, 36.80607986450195f, frame->lightZ};
    const Vector *lightDirection = &aspyrLight;
#else
    const float half = *(const float *)0xa84d2c;
    const float heightScale = *(const float *)0xab8370;
    const float slopeScale = *(const float *)0xab836c;
    const float ambient = *(const float *)0xb4e1b0;
    const Vector *lightDirection = &frame->light;
#endif

    const float altitude = METHOD(terrain, altitudeSlot, Sample)(terrain, x, z);
    const volatile int seaIndex = 64;
    const Color land = map->land[level((altitude - frame->seaLevel) * heightScale + seaIndex, 255)];
    Color water = map->water[0];
    if (wet != dry) {
        // Open water uses the sea plane. Inland water has its own modeled surface.
        const float surface = wet == ocean ? frame->seaLevel :
            METHOD(terrain, waterSurfaceSlot, Sample)(terrain, x, z);
        water = map->water[level(surface - altitude + half, 255)];
    }
    const unsigned alpha = water >> 24;
    float sunlight = lightDirection->y;
    if (altitude >= frame->seaLevel) {
        Vector n = {0, 1, 0};
        METHOD(terrain, normalSlot, Normal)(terrain, x, z, &n);
        n.y *= slopeScale;
        const float length = __builtin_sqrtf(n.x * n.x + n.y * n.y + n.z * n.z);
        if (length > 0)
            sunlight = (lightDirection->x * n.x + lightDirection->y * n.y + lightDirection->z * n.z) / length;
    }
    const unsigned light = level(ambient + (sunlight > 0 ? 2 * sunlight : 0) + half, 256);
    return channel(land, water, light, alpha) |
           (channel(land >> 8, water >> 8, light, alpha) << 8) |
           (channel(land >> 16, water >> 16, light, alpha) << 16);
}

#ifdef SC4_ASPYR
static
#else
__attribute__((section(".text.sample")))
#endif
int renderTerrainChunk(Color *destination, int side, const MiniMap *map, const ChunkFrame *frame) {
    if (!destination || !frame->terrain || map->width != 512 ||
        (side != 32 && side != 64 && side != 128))
        return 0;
    const float cellWidth = METHOD(frame->terrain, cellWidthSlot, Measure)(frame->terrain);
    const volatile int maxCellWidth = 64;
    if (!(cellWidth > 0 && cellWidth <= maxCellWidth))
        return 0;

#ifdef SC4_ASPYR
    const float half = 0.5f;
#else
    const float half = *(const float *)0xa84d2c;
#endif
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

#ifdef SC4_ASPYR
__attribute__((section(".text.aspyr")))
int paintAspyrTerrainChunk(const MiniMap *map, const ChunkFrame *frame) {
    if (map->width != 512 || map->stride != map->width || map->height != map->width ||
        !map->pixels || map->zoomShift < -3 || map->zoomShift > -1)
        return 0;
    const int density = 1 << -map->zoomShift;
    const int side = 16 * density;
    const int lastCell = map->width / density - 16;
    if ((unsigned)frame->x > lastCell || (unsigned)frame->z > lastCell)
        return 0;
    Color *destination = map->pixels + (frame->z * map->stride + frame->x) * density;
    if (renderTerrainChunk(destination, side, map, frame))
        return 1;
    // Aspyr's stock writer table has no 8× writer for a small city.
    if (density != 8)
        return 0;
    expandTerrainChunk(destination, map->stride * 4, frame->tiles, 16 * 4, side, side);
    return 1;
}
#endif
