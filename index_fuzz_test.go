package minpatricia

import (
	"slices"
	"sort"
	"testing"
)

const (
	fuzzMaxOps     = 512
	fuzzCheckEvery = 31
	fuzzMaxKeyLen  = 32
)

func FuzzIndexAgainstMap(f *testing.F) {
	f.Add([]byte{
		0, 5, 'a', 'l', 'p', 'h', 'a',
		0, 5, 'b', 'r', 'a', 'v', 'o',
		0, 5, 'a', 'l', 'p', 'h', 'a',
		2, 5, 'b', 'r', 'a', 'v', 'o',
		3, 5, 'a', 'l', 'p', 'h', 'a',
		4, 1, 'a',
		5, 1, 'z',
		6, 1, 'a', 1, 'z',
		7, 1, 'z', 1, 'a',
	})
	f.Add([]byte{
		0, 0,
		0, 1, 0,
		0, 2, 0, 0,
		0, 3, 0, 0, 0,
		6, 0, 4, 0, 0, 0, 1,
		7, 4, 0, 0, 0, 1, 0,
	})
	f.Add(fuzzSplitSeed(MaxNodeReps + 1))

	f.Fuzz(func(t *testing.T, data []byte) {
		keys := memKeys{}
		idx := NewWithRecords(keys)
		expected := map[string]Position{}
		nextPos := Position(1)
		stream := fuzzStream{data: data}

		for step := 0; step < fuzzMaxOps && stream.more(); step++ {
			op := stream.nextByte()
			key := stream.nextKey()

			switch op % 8 {
			case 0, 1:
				fuzzPut(t, idx, keys, expected, &nextPos, key)
			case 2:
				fuzzDelete(t, idx, expected, key)
			case 3:
				fuzzGet(t, idx, expected, key)
			case 4:
				fuzzAscendGreaterOrEqual(t, idx, expected, key)
			case 5:
				fuzzDescendLessOrEqual(t, idx, expected, key)
			case 6:
				fuzzAscendRange(t, idx, expected, key, stream.nextKey())
			case 7:
				fuzzDescendRange(t, idx, expected, key, stream.nextKey())
			}

			if step%fuzzCheckEvery == 0 {
				assertIndexMatchesMap(t, idx, keys, expected)
			}
		}

		assertIndexMatchesMap(t, idx, keys, expected)
	})
}

type fuzzStream struct {
	data []byte
	off  int
}

func (s *fuzzStream) more() bool {
	return s.off < len(s.data)
}

func (s *fuzzStream) nextByte() byte {
	b := s.data[s.off]
	s.off++
	return b
}

func (s *fuzzStream) nextKey() []byte {
	if !s.more() {
		return nil
	}
	n := int(s.nextByte() % (fuzzMaxKeyLen + 1))
	if n > len(s.data)-s.off {
		n = len(s.data) - s.off
	}
	key := append([]byte(nil), s.data[s.off:s.off+n]...)
	s.off += n
	return key
}

func fuzzPut(t *testing.T, idx *Index, keys memKeys, expected map[string]Position, nextPos *Position, key []byte) {
	t.Helper()

	pos := *nextPos
	*nextPos = pos + 1
	keys[pos] = key

	old, replaced, err := idx.Put(key, pos)
	if err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}

	wantOld, exists := expected[string(key)]
	if replaced != exists || old != wantOld {
		t.Fatalf("Put(%q) = old %d replaced %v, want old %d replaced %v", key, old, replaced, wantOld, exists)
	}
	expected[string(key)] = pos
}

func fuzzDelete(t *testing.T, idx *Index, expected map[string]Position, key []byte) {
	t.Helper()

	want, exists := expected[string(key)]
	got, deleted, err := idx.Delete(key)
	if err != nil {
		t.Fatalf("Delete(%q): %v", key, err)
	}
	if deleted != exists || got != want {
		t.Fatalf("Delete(%q) = pos %d deleted %v, want pos %d deleted %v", key, got, deleted, want, exists)
	}
	if !exists {
		return
	}

	delete(expected, string(key))
	if got, ok, err := idx.Get(key); err != nil || ok || got != 0 {
		t.Fatalf("Get(%q) after delete = pos %d ok %v err %v, want 0 false nil", key, got, ok, err)
	}
}

func fuzzGet(t *testing.T, idx *Index, expected map[string]Position, key []byte) {
	t.Helper()

	want, exists := expected[string(key)]
	got, ok, err := idx.Get(key)
	if err != nil {
		t.Fatalf("Get(%q): %v", key, err)
	}
	if ok != exists || got != want {
		t.Fatalf("Get(%q) = pos %d ok %v, want pos %d ok %v", key, got, ok, want, exists)
	}
}

func fuzzAscendGreaterOrEqual(t *testing.T, idx *Index, expected map[string]Position, pivot []byte) {
	t.Helper()

	sortedKeys := fuzzSortedKeys(expected)
	wantIdx := sort.SearchStrings(sortedKeys, string(pivot))
	gotKey, gotPos, gotOK := fuzzFirst(t, "AscendGreaterOrEqual", func(fn ItemIterator) error {
		return idx.AscendGreaterOrEqual(pivot, fn)
	})
	if wantIdx == len(sortedKeys) {
		if gotOK {
			t.Fatalf("AscendGreaterOrEqual(%q) = %q, want no item", pivot, gotKey)
		}
		return
	}

	wantKey := sortedKeys[wantIdx]
	if !gotOK || gotKey != wantKey || gotPos != expected[wantKey] {
		t.Fatalf("AscendGreaterOrEqual(%q) = (%q,%d,%v), want (%q,%d,true)", pivot, gotKey, gotPos, gotOK, wantKey, expected[wantKey])
	}
}

func fuzzDescendLessOrEqual(t *testing.T, idx *Index, expected map[string]Position, pivot []byte) {
	t.Helper()

	sortedKeys := fuzzSortedKeys(expected)
	pivotString := string(pivot)
	wantIdx := sort.Search(len(sortedKeys), func(i int) bool {
		return sortedKeys[i] > pivotString
	}) - 1
	gotKey, gotPos, gotOK := fuzzFirst(t, "DescendLessOrEqual", func(fn ItemIterator) error {
		return idx.DescendLessOrEqual(pivot, fn)
	})
	if wantIdx < 0 {
		if gotOK {
			t.Fatalf("DescendLessOrEqual(%q) = %q, want no item", pivot, gotKey)
		}
		return
	}

	wantKey := sortedKeys[wantIdx]
	if !gotOK || gotKey != wantKey || gotPos != expected[wantKey] {
		t.Fatalf("DescendLessOrEqual(%q) = (%q,%d,%v), want (%q,%d,true)", pivot, gotKey, gotPos, gotOK, wantKey, expected[wantKey])
	}
}

func fuzzAscendRange(t *testing.T, idx *Index, expected map[string]Position, greaterOrEqual []byte, lessThan []byte) {
	t.Helper()

	lo := string(greaterOrEqual)
	hi := string(lessThan)
	var want []string
	for _, key := range fuzzSortedKeys(expected) {
		if key >= lo && key < hi {
			want = append(want, key)
		}
	}

	got := fuzzCollect(t, "AscendRange", func(fn ItemIterator) error {
		return idx.AscendRange(greaterOrEqual, lessThan, fn)
	})
	if !slices.Equal(got, want) {
		t.Fatalf("AscendRange(%q,%q) = %q, want %q", greaterOrEqual, lessThan, got, want)
	}
}

func fuzzDescendRange(t *testing.T, idx *Index, expected map[string]Position, lessOrEqual []byte, greaterThan []byte) {
	t.Helper()

	hi := string(lessOrEqual)
	lo := string(greaterThan)
	sortedKeys := fuzzSortedKeys(expected)
	var want []string
	for i := len(sortedKeys) - 1; i >= 0; i-- {
		key := sortedKeys[i]
		if key <= hi && key > lo {
			want = append(want, key)
		}
	}

	got := fuzzCollect(t, "DescendRange", func(fn ItemIterator) error {
		return idx.DescendRange(lessOrEqual, greaterThan, fn)
	})
	if !slices.Equal(got, want) {
		t.Fatalf("DescendRange(%q,%q) = %q, want %q", lessOrEqual, greaterThan, got, want)
	}
}

func fuzzFirst(t *testing.T, name string, run func(ItemIterator) error) (string, Position, bool) {
	t.Helper()

	var gotKey string
	var gotPos Position
	var gotOK bool
	err := run(func(key []byte, pos Position) bool {
		gotKey = string(key)
		gotPos = pos
		gotOK = true
		return false
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return gotKey, gotPos, gotOK
}

func fuzzCollect(t *testing.T, name string, run func(ItemIterator) error) []string {
	t.Helper()

	var got []string
	err := run(func(key []byte, _ Position) bool {
		got = append(got, string(key))
		return true
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return got
}

func fuzzSortedKeys(expected map[string]Position) []string {
	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fuzzSplitSeed(count int) []byte {
	seed := make([]byte, 0, count*4)
	for i := 0; i < count; i++ {
		seed = append(seed, 0, 2, byte(i>>8), byte(i))
	}
	return seed
}
