package minpatricia

// RecordStore maps an opaque record position back to its key.
//
// Index stores only Positions and child node IDs. It never owns keys or
// payloads; callers keep records in their own store and use this interface to
// let the trie compare candidate records.
type RecordStore interface {
	Key(pos Position) ([]byte, bool)
}

type RecordStoreFunc func(pos Position) ([]byte, bool)

func (f RecordStoreFunc) Key(pos Position) ([]byte, bool) {
	return f(pos)
}

type HeapRecord[V any] struct {
	Key   []byte
	Value V
}

type HeapRecordStore[V any] struct {
	records []HeapRecord[V]
	free    []Position
	live    int
}

func NewHeapRecordStore[V any]() *HeapRecordStore[V] {
	return &HeapRecordStore[V]{
		records: make([]HeapRecord[V], 1),
	}
}

var emptyHeapRecordKey = []byte{}

// Add stores key and value and returns their opaque Position.
//
// HeapRecordStore uses nil key as the free-slot sentinel, so nil input is
// normalized to an empty key.
func (s *HeapRecordStore[V]) Add(key []byte, value V) Position {
	key = heapRecordKey(key)
	if s.records == nil {
		s.records = make([]HeapRecord[V], 1)
	}
	if len(s.free) != 0 {
		last := len(s.free) - 1
		pos := s.free[last]
		s.free[last] = 0
		s.free = s.free[:last]
		s.records[pos] = HeapRecord[V]{
			Key:   key,
			Value: value,
		}
		s.live++
		return pos
	}

	pos := Position(len(s.records))
	s.records = append(s.records, HeapRecord[V]{
		Key:   key,
		Value: value,
	})
	s.live++
	return pos
}

func heapRecordKey(key []byte) []byte {
	if key == nil {
		return emptyHeapRecordKey
	}
	return key
}

func (s *HeapRecordStore[V]) Free(pos Position) error {
	if pos == 0 || uint64(pos) >= uint64(len(s.records)) || s.records[pos].Key == nil {
		return ErrMissingKey
	}
	s.records[pos] = HeapRecord[V]{}
	s.free = append(s.free, pos)
	s.live--
	return nil
}

func (s *HeapRecordStore[V]) Key(pos Position) ([]byte, bool) {
	if pos == 0 || uint64(pos) >= uint64(len(s.records)) {
		return nil, false
	}
	record := &s.records[pos]
	if record.Key == nil {
		return nil, false
	}
	return record.Key, true
}

func (s *HeapRecordStore[V]) Value(pos Position) (V, bool) {
	var zero V
	if pos == 0 || uint64(pos) >= uint64(len(s.records)) {
		return zero, false
	}
	record := &s.records[pos]
	if record.Key == nil {
		return zero, false
	}
	return record.Value, true
}

func (s *HeapRecordStore[V]) Record(pos Position) (HeapRecord[V], bool) {
	if pos == 0 || uint64(pos) >= uint64(len(s.records)) {
		return HeapRecord[V]{}, false
	}
	record := s.records[pos]
	if record.Key == nil {
		return HeapRecord[V]{}, false
	}
	return record, true
}

func (s *HeapRecordStore[V]) Len() int {
	return s.live
}
