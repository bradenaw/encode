// Package encode provides utilities for encoding and decoding structures into raw bytes.
//
// Normal usage of this package looks like this:
//
//   type encodableFoo struct {
//   	a uint16
//   	b string
//   	c bool
//   }
//
//   func (e encodableFoo) encoding() encode.Encoding {
//   	return encode.New(
//   		encode.BigEndianUint16(&e.a),
//   		encode.LengthDelimString(&e.b),
//   		encode.Bool(&e.c),
//   	)
//   }
//
//   func (e encodableFoo) Encode() []byte {
//   	return e.encoding().Encode()
//   }
//
//   func (e encodableFoo) Decode(b []byte) error {
//   	return e.encoding().Decode(b)
//   }
package encode

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/bits"
)

var errOverflowVarint = errors.New("encode: invalid varint")

type Item interface {
	// Encode this item into buf. buf will be at least Size() bytes.
	Encode(buf []byte)
	// Decode buf into this item, mutating it to match the representation in buf.
	Decode(buf []byte) error
	// The number of bytes that Encode() will use.
	Size() int
}

type Encoding struct {
	items []Item
}

func New(items ...Item) Encoding {
	return Encoding{items: items}
}

func (enc Encoding) Encode() []byte {
	totalSize := 0
	for _, item := range enc.items {
		totalSize += item.Size()
	}
	buf := make([]byte, totalSize)
	i := 0
	for _, item := range enc.items {
		size := item.Size()
		item.Encode(buf[i : i+size])
		i += size
	}
	return buf
}

func (enc Encoding) Decode(buf []byte) error {
	i := 0
	for _, item := range enc.items {
		err := item.Decode(buf[i:])
		if err != nil {
			return err
		}
		i += item.Size()
	}
	return nil
}

// Quietly ignore n bytes.
func Padding(n int) Item {
	return padding{n}
}

type padding struct{ n int }

func (e padding) Encode(buf []byte) {}
func (e padding) Size() int {
	return e.n
}
func (e padding) Decode(buf []byte) error {
	if len(buf) < e.n {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// Encode v as itself.
func Byte(v *byte) Item {
	return encByte{v}
}

type encByte struct{ v *byte }

func (e encByte) Encode(buf []byte) {
	buf[0] = *e.v
}
func (e encByte) Size() int {
	return 1
}
func (e encByte) Decode(buf []byte) error {
	if len(buf) < 1 {
		return io.ErrUnexpectedEOF
	}
	*e.v = buf[0]
	return nil
}

// Encode v as 0x01 (true) or 0x00 (false).
func Bool(v *bool) Item {
	return encBool{v}
}

type encBool struct{ v *bool }

func (e encBool) Encode(buf []byte) {
	if *e.v {
		buf[0] = 1
	}
}
func (e encBool) Size() int {
	return 1
}
func (e encBool) Decode(buf []byte) error {
	if len(buf) < 1 {
		return io.ErrUnexpectedEOF
	}
	*e.v = buf[0] == 1
	return nil
}

// Encode v in big endian order, taking 2 bytes.
func BigEndianUint16(v *uint16) Item {
	return bigEndianUint16{v}
}

type bigEndianUint16 struct{ v *uint16 }

func (e bigEndianUint16) Encode(buf []byte) {
	binary.BigEndian.PutUint16(buf, *e.v)
}
func (e bigEndianUint16) Size() int {
	return 2
}
func (e bigEndianUint16) Decode(buf []byte) error {
	if len(buf) < 2 {
		return io.ErrUnexpectedEOF
	}
	*e.v = binary.BigEndian.Uint16(buf)
	return nil
}

// Encode v in big endian order, taking 4 bytes.
func BigEndianUint32(v *uint32) Item {
	return bigEndianUint32{v}
}

type bigEndianUint32 struct{ v *uint32 }

func (e bigEndianUint32) Encode(buf []byte) {
	binary.BigEndian.PutUint32(buf, *e.v)
}
func (e bigEndianUint32) Size() int {
	return 4
}
func (e bigEndianUint32) Decode(buf []byte) error {
	if len(buf) < 4 {
		return io.ErrUnexpectedEOF
	}
	*e.v = binary.BigEndian.Uint32(buf)
	return nil
}

// Encode v in big endian order, taking 8 bytes.
func BigEndianUint64(v *uint64) Item {
	return bigEndianUint64{v}
}

type bigEndianUint64 struct{ v *uint64 }

func (e bigEndianUint64) Encode(buf []byte) {
	binary.BigEndian.PutUint64(buf, *e.v)
}
func (e bigEndianUint64) Size() int {
	return 8
}
func (e bigEndianUint64) Decode(buf []byte) error {
	if len(buf) < 8 {
		return io.ErrUnexpectedEOF
	}
	*e.v = binary.BigEndian.Uint64(buf)
	return nil
}

// Encode v using a variable-length encoding, so that smaller numbers use fewer bytes.
//
// See more at https://developers.google.com/protocol-buffers/docs/encoding#varints
//
//   input bits
//   high order             low order
//   uuuuwwwwwwwzzzzzzzyyyyyyyxxxxxxx
//
//   min     max          encoded size     encoding
//   0       2^7 - 1      1                0xxxxxxx
//   2^7     2^14 - 1     2                1xxxxxxx 0yyyyyyy
//   2^14    2^21 - 1     3                1xxxxxxx 1yyyyyyy 0zzzzzzz
//   2^21    2^28 - 1     4                1xxxxxxx 1yyyyyyy 1zzzzzzz 0wwwwwww
//   2^28    2^32 - 1     5                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 0000uuuu
func Uvarint32(v *uint32) Item {
	return uvarint32{v}
}

type uvarint32 struct{ v *uint32 }

func (e uvarint32) Encode(buf []byte) {
	binary.PutUvarint(buf, uint64(*e.v))
}
func (e uvarint32) Size() int {
	return uvarintSize(uint64(*e.v))
}
func (e uvarint32) Decode(buf []byte) error {
	l, n := binary.Uvarint(buf)
	if n == 0 {
		return io.ErrUnexpectedEOF
	}
	if n < 0 {
		return errOverflowVarint
	}
	if n > math.MaxUint32 {
		return errOverflowVarint
	}
	*e.v = uint32(l)
	return nil
}

// Encode v using a variable-length encoding, so that smaller numbers use fewer bytes.
//
// See more at https://developers.google.com/protocol-buffers/docs/encoding#varints
//
//   input bits
//   high order                                             low order
//   ddcccccccbbbbbbbaaaaaaavvvvvvvuuuuuuwwwwwwwzzzzzzzyyyyyyyxxxxxxx
//
//   min     max          encoded size     encoding
//   0       2^7 - 1      1                0xxxxxxx
//   2^7     2^14 - 1     2                1xxxxxxx 0yyyyyyy
//   2^14    2^21 - 1     3                1xxxxxxx 1yyyyyyy 0zzzzzzz
//   2^21    2^28 - 1     4                1xxxxxxx 1yyyyyyy 1zzzzzzz 0wwwwwww
//   2^28    2^35 - 1     5                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 0uuuuuuu
//   2^35    2^42 - 1     6                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 1uuuuuuu 0vvvvvvv
//   2^42    2^49 - 1     7                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 1uuuuuuu 1vvvvvvv 0aaaaaaa
//   2^49    2^56 - 1     8                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 1uuuuuuu 1vvvvvvv 1aaaaaaa 0bbbbbbb
//   2^56    2^63 - 1     9                1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 1uuuuuuu 1vvvvvvv 1aaaaaaa 1bbbbbbb 0ccccccc
//   2^63    2^64 - 1     10               1xxxxxxx 1yyyyyyy 1zzzzzzz 1wwwwwww 1uuuuuuu 1vvvvvvv 1aaaaaaa 1bbbbbbb 1ccccccc 000000dd
func Uvarint64(v *uint64) Item {
	return uvarint64{v}
}

type uvarint64 struct{ v *uint64 }

func (e uvarint64) Encode(buf []byte) {
	binary.PutUvarint(buf, *e.v)
}
func (e uvarint64) Size() int {
	return uvarintSize(*e.v)
}
func (e uvarint64) Decode(buf []byte) error {
	l, n := binary.Uvarint(buf)
	if n == 0 {
		return io.ErrUnexpectedEOF
	}
	if n < 0 {
		return errOverflowVarint
	}
	*e.v = l
	return nil
}

// Similar to Uvarint64, produces a variable-length encoding for v. However, it has two advantages:
// it preserves ordering, in that the encoded bytes will lexicographically order the same as the
// inputs would be ordered numerically; and it uses one fewer byte for numbers larger than 2^63-1.
//
// It does this by using a similar technique to UTF-8 (see
// https://en.wikipedia.org/wiki/UTF-8#Examples), where only significant bits are encoded, and a
// number of leading ones is used to determine the length of the encoding.
//
//   min     max          encoded size     encoding, where x is an input bit
//   0       2^7 - 1      1                0xxxxxxx
//   2^7     2^14 - 1     2                10xxxxxx xxxxxxxx
//   2^14    2^21 - 1     3                110xxxxx xxxxxxxx xxxxxxxx
//   2^21    2^28 - 1     4                1110xxxx xxxxxxxx xxxxxxxx xxxxxxxx
//   2^28    2^35 - 1     5                11110xxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
//   2^35    2^42 - 1     6                111110xx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
//   2^42    2^49 - 1     7                1111110x xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
//   2^49    2^56 - 1     8                11111110 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
//   2^56    2^64 - 1     9                11111111 xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx xxxxxxxx
func OrdUvarint64(v *uint64) Item {
	return ordUvarint64{v}
}

type ordUvarint64 struct{ v *uint64 }

func (e ordUvarint64) Encode(buf []byte) {
	l := bits.Len64(*e.v)
	if l > 56 {
		buf[0] = 0xFF
		binary.BigEndian.PutUint64(buf[1:], *e.v)
		return
	}

	nBytes := (l + 6) / 7
	nLeadingOnes := nBytes - 1
	buf[0] = ((1 << uint(nLeadingOnes)) - 1) << uint(8-nLeadingOnes)
	for i := 0; i < nBytes; i++ {
		buf[i] |= byte(*e.v >> uint((nBytes-i-1)*8))
	}
}
func (e ordUvarint64) Size() int {
	l := bits.Len64(*e.v)
	if l > 56 {
		return 9
	}
	return 1 + (l-1)/7
}
func (e ordUvarint64) Decode(buf []byte) error {
	if len(buf) < 1 {
		return io.ErrUnexpectedEOF
	}
	nLeadingOnes := bits.LeadingZeros8(^buf[0])
	nBytes := nLeadingOnes + 1
	rBits := nBytes * 7
	rBytes := (rBits + 8) / 8

	if rBits == 63 {
		if len(buf) < 9 {
			return io.ErrUnexpectedEOF
		}
		*e.v = binary.BigEndian.Uint64(buf[1:])
		return nil
	}

	if len(buf) < nBytes {
		return io.ErrUnexpectedEOF
	}
	result := uint64(0)
	for i := 0; i < nBytes; i++ {
		shift := (rBytes * 8) - (i * 8) - 8
		result |= uint64(buf[i]) << uint(shift)
	}
	mask := (uint64(1) << uint(rBits)) - 1
	*e.v = result & mask
	return nil
}

// Encode v as a uvarint of v's length, followed by v.
func LengthDelimBytes(v *[]byte) Item {
	return lengthDelimBytes{v}
}

type lengthDelimBytes struct{ v *[]byte }

func (e lengthDelimBytes) Encode(buf []byte) {
	n := binary.PutUvarint(buf, uint64(len(*e.v)))
	copy(buf, (*e.v)[n:])
}
func (e lengthDelimBytes) Size() int {
	return uvarintSize(uint64(len(*e.v))) + len(*e.v)
}
func (e lengthDelimBytes) Decode(buf []byte) error {
	l, n := binary.Uvarint(buf)
	if n == 0 {
		return io.ErrUnexpectedEOF
	}
	if n < 0 {
		return errOverflowVarint
	}
	if uint64(len(buf[n:])) < l {
		return io.ErrUnexpectedEOF
	}
	*e.v = make([]byte, l)
	copy(buf[n:], *e.v)
	return nil
}

// Encode v as a uvarint of v's length, followed by v.
func LengthDelimString(v *string) Item {
	return lengthDelimString{v}
}

type lengthDelimString struct{ v *string }

func (e lengthDelimString) Encode(buf []byte) {
	n := binary.PutUvarint(buf, uint64(len(*e.v)))
	copy(buf, (*e.v)[n:])
}
func (e lengthDelimString) Size() int {
	return uvarintSize(uint64(len(*e.v))) + len(*e.v)
}
func (e lengthDelimString) Decode(buf []byte) error {
	l, n := binary.Uvarint(buf)
	if n == 0 {
		return io.ErrUnexpectedEOF
	}
	if n < 0 {
		return errOverflowVarint
	}
	if uint64(len(buf[n:])) < l {
		return io.ErrUnexpectedEOF
	}
	*e.v = string(buf[n:])
	return nil
}

// Encode a fixed-length 16 bytes directly.
func Bytes16(v *[16]byte) Item {
	return bytes16{v}
}

type bytes16 struct{ v *[16]byte }

func (e bytes16) Encode(buf []byte) {
	copy(buf, (*e.v)[:])
}
func (e bytes16) Size() int {
	return 16
}
func (e bytes16) Decode(buf []byte) error {
	if len(buf) < 16 {
		return io.ErrUnexpectedEOF
	}
	copy((*e.v)[:], buf[:16])
	return nil
}

// Encode a fixed-length 32 bytes directly.
func Bytes32(v *[32]byte) Item {
	return bytes32{v}
}

type bytes32 struct{ v *[32]byte }

func (e bytes32) Encode(buf []byte) {
	copy(buf, (*e.v)[:])
}
func (e bytes32) Size() int {
	return 32
}
func (e bytes32) Decode(buf []byte) error {
	if len(buf) < 32 {
		return io.ErrUnexpectedEOF
	}
	copy((*e.v)[:], buf[:32])
	return nil
}

func uvarintSize(x uint64) int {
	var b [binary.MaxVarintLen64]byte
	return binary.PutUvarint(b[:], x)
}

func varintSize(x int64) int {
	var b [binary.MaxVarintLen64]byte
	return binary.PutVarint(b[:], x)
}
