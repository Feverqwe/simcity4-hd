//go:build cgo

package sc4

/*
// Volatile reads keep the compiler from folding the reconstructed key into a literal.
static const volatile char scrambled[] = "a7fbf8d8b1d73c22b75de0214c4d4a118d4073b75e5246f9a32d4b6247c4e416";

static unsigned position(unsigned index) {
    static const unsigned multipliers[] = {5, 3, 7, 5, 3};
    static const unsigned salts[] = {11, 19, 23, 29, 37};
    unsigned left = index >> 3;
    unsigned right = index & 7;
    for (unsigned round = 0; round < sizeof(salts) / sizeof(salts[0]); ++round) {
        const unsigned mixed = right * multipliers[round] + salts[round];
        const unsigned next = left ^ ((mixed ^ (mixed >> 3)) & 7);
        left = right;
        right = next;
    }
    return (left << 3) | right;
}

static unsigned nibble(unsigned index) {
    const char c = scrambled[position(index)];
    return c <= '9' ? c - '0' : c - 'a' + 10;
}

static void sc4_russian_pack_key(unsigned char *key) {
    for (unsigned i = 0; i < 32; ++i) {
        key[i] = (unsigned char)((nibble(2 * i) << 4) | nibble(2 * i + 1));
    }
}
*/
import "C"

import "unsafe"

func russianPackKey() (key [32]byte, err error) {
	C.sc4_russian_pack_key((*C.uchar)(unsafe.Pointer(&key[0])))
	return key, nil
}
