package minpatricia

import (
	"bytes"
	"errors"
	"math/bits"
)

const (
	maxDiff    = 1<<16 - 1
	MaxKeySize = maxDiff / 9
)

var (
	ErrEqualKeys      = errors.New("minpatricia: equal keys have no diff bit")
	ErrKeyTooLarge    = errors.New("minpatricia: key is too large for uint16 diff")
	ErrUnsortedKeys   = errors.New("minpatricia: reps are not sorted by key")
	ErrMissingKey     = errors.New("minpatricia: record store has no key for position")
	ErrPositionTag    = errors.New("minpatricia: record position uses reserved high bit")
	ErrPositionKey    = errors.New("minpatricia: record position key does not match input key")
	ErrDuplicateKey   = errors.New("minpatricia: duplicate key in node")
	ErrCorruptLayout  = errors.New("minpatricia: corrupt node layout")
	ErrNilRecordStore = errors.New("minpatricia: nil record store")
	ErrNilNodeStore   = errors.New("minpatricia: nil node store")
)

func checkKeySize(key []byte) error {
	if len(key) > MaxKeySize {
		return ErrKeyTooLarge
	}
	return nil
}

func compareKeys(a, b []byte) int {
	return bytes.Compare(a, b)
}

func findDiffBit(a, b []byte) (uint16, error) {
	_, diff, err := compareAndDiffBit(a, b)
	return diff, err
}

func compareAndDiffBit(a, b []byte) (int, uint16, error) {
	if err := checkKeySize(a); err != nil {
		return 0, 0, err
	}
	if err := checkKeySize(b); err != nil {
		return 0, 0, err
	}

	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	i := 0
	for i < n && a[i] == b[i] {
		i++
	}

	if i == n {
		if len(a) == len(b) {
			return 0, 0, ErrEqualKeys
		}
		diff := uint16(i * 9)
		if len(a) < len(b) {
			return -1, diff, nil
		}
		return 1, diff, nil
	}

	x := a[i] ^ b[i]
	diff := uint16(i*9 + bits.LeadingZeros8(x) + 1)
	if a[i] < b[i] {
		return -1, diff, nil
	}
	return 1, diff, nil
}

func getDiffBit(key []byte, diff uint16) uint8 {
	byteIdx := int(diff / 9)
	bitIdx := uint(diff % 9)
	if bitIdx == 0 {
		if byteIdx < len(key) {
			return 1
		}
		return 0
	}
	if byteIdx >= len(key) {
		return 0
	}
	return (key[byteIdx] >> (8 - bitIdx)) & 1
}
