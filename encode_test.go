package encode

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/bradenaw/trand"
	"github.com/stretchr/testify/require"
)

func TestOrdUvarint64(t *testing.T) {
	checkRoundtrip := func(x uint64) {
		enc := New(OrdUvarint64(&x))
		x2 := x
		b := enc.Encode()
		t.Logf("%d: %s\n", x, hex.EncodeToString(b))
		x = ^x
		err := enc.Decode(b)
		require.NoError(t, err)
		require.Equal(t, x2, x)
	}

	checkOrdering := func(x uint64, x2 uint64) {
		checkRoundtrip(x)
		checkRoundtrip(x2)

		enc := New(OrdUvarint64(&x))
		b := enc.Encode()

		enc2 := New(OrdUvarint64(&x2))
		b2 := enc2.Encode()

		require.True(
			t,
			bytes.Compare(b, b2) < 0,
			"%d < %d but %s >= %s",
			x, x2, hex.EncodeToString(b), hex.EncodeToString(b2),
		)
	}

	checkOrdering(0, 1)
	checkOrdering(1, 14)
	checkOrdering(16, 123)
	checkOrdering(16, 128)
	checkOrdering(1235, 1239)
	checkOrdering(1231, 123151)
	checkOrdering(1231241, 1230123102)
	checkOrdering(1<<7, 1<<14)
	checkOrdering(1<<14, 1<<21)
	checkOrdering(1<<21, 1<<28)
	checkOrdering(1<<28, 1<<35)
	checkOrdering(1<<35, 1<<42)
	checkOrdering(1<<42, 1<<49)
	checkOrdering(1<<49, 1<<56)
	checkOrdering(1<<56, 1<<63)
	checkOrdering(1<<63, 1<<63+15)
	checkOrdering(1<<63+15, 1<<63+1<<62)
	checkOrdering(1231241, 1<<63+1231023105915)
	checkOrdering(1<<63+1<<62, ^uint64(0))

	trand.RandomN(t, 10000, func(t *testing.T, r *rand.Rand) {
		x1 := r.Uint64() >> uint(r.Int()%64)
		x2 := r.Uint64() >> uint(r.Int()%64)

		if x1 == x2 {
			x2++
		}
		if x2 < x1 {
			x1, x2 = x2, x1
		}

		checkOrdering(x1, x2)
	})
}

func TestOrdVarint64(t *testing.T) {
	checkEncoding := func(x int64, expected []byte) {
		enc := New(OrdVarint64(&x))
		b := enc.Encode()
		require.Equal(t, expected, b)
	}
	checkRoundtrip := func(x int64) {
		enc := New(OrdVarint64(&x))
		x2 := x
		b := enc.Encode()
		t.Logf("%d: %s\n", x, hex.EncodeToString(b))
		x = ^x
		err := enc.Decode(b)
		require.NoError(t, err)
		require.Equal(t, x2, x)
	}

	checkOrdering := func(x int64, x2 int64) {
		checkRoundtrip(x)
		checkRoundtrip(x2)

		enc := New(OrdVarint64(&x))
		b := enc.Encode()

		enc2 := New(OrdVarint64(&x2))
		b2 := enc2.Encode()

		require.True(
			t,
			bytes.Compare(b, b2) < 0,
			"%d < %d but %s >= %s",
			x, x2, hex.EncodeToString(b), hex.EncodeToString(b2),
		)
	}

	checkEncoding(int64(-(1 << 63)), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	checkEncoding(0, []byte{0x80})
	checkEncoding(1, []byte{0x81})
	checkEncoding(-1, []byte{0x7F})
	checkEncoding(-2, []byte{0x7E})
	checkEncoding(-64, []byte{0x40})
	checkEncoding(-65, []byte{0x3F, 0xBF}) // 00111111 10111111
	checkEncoding(64, []byte{0xC0, 0x40})  // 11000000 01000000
	checkEncoding(int64(1<<63-1), []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	checkOrdering(-125, -5)
	checkOrdering(-65, -1)
	checkOrdering(-64, -1)
	checkOrdering(-32, -1)
	checkOrdering(-576, 15)

	checkOrdering(0, 1)
	checkOrdering(1, 14)
	checkOrdering(16, 123)
	checkOrdering(16, 128)
	checkOrdering(1235, 1239)
	checkOrdering(1231, 123151)
	checkOrdering(1231241, 1230123102)
	checkOrdering(1<<7, 1<<14)
	checkOrdering(1<<14, 1<<21)
	checkOrdering(1<<21, 1<<28)
	checkOrdering(1<<28, 1<<35)
	checkOrdering(1<<35, 1<<42)
	checkOrdering(1<<42, 1<<49)
	checkOrdering(1<<49, 1<<56)
	checkOrdering(1<<56, 1<<63-1)
	checkOrdering(-1<<6, 1<<6)
	checkOrdering(-1<<13, 1<<13-1)

	trand.RandomN(t, 10000, func(t *testing.T, r *rand.Rand) {
		x1 := int64(r.Uint64()) >> uint(r.Int()%64)
		x2 := int64(r.Uint64()) >> uint(r.Int()%64)

		if x1 == x2 {
			x2++
		}
		if x2 < x1 {
			x1, x2 = x2, x1
		}

		checkOrdering(x1, x2)
	})
}

func BenchmarkOrdUvarint64Encode(b *testing.B) {
	bunchaUint64s := make([]uint64, b.N)
	for i := range bunchaUint64s {
		bunchaUint64s[i] = rand.Uint64() >> uint(rand.Int()%64)
	}

	b.ResetTimer()

	var backing [9]byte
	for i := 0; i < b.N; i++ {
		x := bunchaUint64s[i%len(bunchaUint64s)]
		enc := ordUvarint64{&x}
		enc.Encode(backing[:enc.Size()])
	}
}

func BenchmarkOrdUvarint64Decode(b *testing.B) {
	bunchaEncoded := make([][]byte, b.N)
	for i := range bunchaEncoded {
		x := rand.Uint64() >> uint(rand.Int()%64)
		enc := ordUvarint64{&x}
		buf := make([]byte, enc.Size())
		enc.Encode(buf)
		bunchaEncoded[i] = buf
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var x uint64
		enc := ordUvarint64{&x}
		_ = enc.Decode(bunchaEncoded[i%len(bunchaEncoded)])
	}
}
