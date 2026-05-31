package minpatricia

// Index is an ordered trie over opaque record positions.
//
// The trie stores neither keys nor payloads. Records live in a caller-owned
// store; Index keeps only Position handles and asks RecordStore to recover keys
// when it needs to compare, verify, or iterate records. Child links are stable
// node IDs resolved through a NodeStore, so the same logical tree can be
// backed by heap nodes today and mmap pages later.
type Index struct {
	records RecordStore
	nodes   NodeStore
	rootID  uint64
	count   int
	scratch rebuildScratch
}

func NewHeap[V any]() (*Index, *HeapRecordStore[V]) {
	records := NewHeapRecordStore[V]()
	return NewWithRecords(records), records
}

func NewWithRecords(records RecordStore) *Index {
	return NewWithNodes(records, NewHeapNodeStore())
}

func NewWithNodes(records RecordStore, nodes NodeStore) *Index {
	idx, err := newIndex(records, nodes, true)
	if err != nil {
		panic(err)
	}
	return idx
}

// OpenWithNodes opens an existing NodeStore without rebuilding root.
//
// This is the constructor a future mmap-backed implementation should use after
// mapping persisted node pages. NewWithNodes is for fresh mutable trees.
func OpenWithNodes(records RecordStore, nodes NodeStore) (*Index, error) {
	return newIndex(records, nodes, false)
}

func newIndex(records RecordStore, nodes NodeStore, initRoot bool) (*Index, error) {
	if records == nil {
		return nil, ErrNilRecordStore
	}
	if nodes == nil {
		return nil, ErrNilNodeStore
	}

	idx := &Index{
		records: records,
		nodes:   nodes,
		rootID:  nodes.Root(),
	}
	root, err := idx.nodeByID(idx.rootID)
	if err != nil {
		return nil, err
	}
	if initRoot {
		if err := idx.rebuildNode(root); err != nil {
			return nil, err
		}
	}
	count, err := idx.countRecords(root)
	if err != nil {
		return nil, err
	}
	idx.count = count
	return idx, nil
}

func (idx *Index) Len() int {
	return idx.count
}

func (idx *Index) Get(key []byte) (Position, bool, error) {
	pos, ok, err := idx.Probe(key)
	if err != nil || !ok {
		return pos, ok, err
	}

	recordKey, err := idx.key(pos)
	if err != nil {
		return 0, false, err
	}
	if compareKeys(recordKey, key) != 0 {
		return 0, false, nil
	}
	return pos, true, nil
}

// Probe returns the record position reached by routing key through the trie.
//
// Probe does not read RecordStore and therefore does not verify that the
// returned position's key exactly matches key. Callers that only need to compare
// the routed position can use it to avoid the extra key lookup done by Get.
func (idx *Index) Probe(key []byte) (Position, bool, error) {
	if err := checkKeySize(key); err != nil {
		return 0, false, err
	}

	n, err := idx.root()
	if err != nil {
		return 0, false, err
	}
	for {
		leaf, ok := n.lookup(key)
		if !ok {
			return 0, false, nil
		}

		r := n.reps[leaf]
		if r.isChild() {
			child, err := idx.nodeByID(r.childID())
			if err != nil {
				return 0, false, err
			}
			n = child
			continue
		}

		return r.position(), true, nil
	}
}

// Retarget replaces the routed record position when Probe(key) equals oldPos.
//
// Retarget only walks index nodes. It does not read RecordStore and therefore
// assumes newPos refers to the same key as oldPos.
func (idx *Index) Retarget(key []byte, oldPos, newPos Position) error {
	if err := checkKeySize(key); err != nil {
		return err
	}

	newRep, err := makeRecordRep(newPos)
	if err != nil {
		return err
	}
	return idx.retarget(key, oldPos, newRep)
}

func (idx *Index) Put(key []byte, pos Position) (Position, bool, error) {
	if err := checkKeySize(key); err != nil {
		return 0, false, err
	}

	recordKey, err := idx.key(pos)
	if err != nil {
		return 0, false, err
	}
	if compareKeys(recordKey, key) != 0 {
		return 0, false, ErrPositionKey
	}

	newRep, err := makeRecordRep(pos)
	if err != nil {
		return 0, false, err
	}

	old, replaced, err := idx.insertOrReplace(key, newRep)
	if err != nil {
		return 0, false, err
	}
	if !replaced {
		idx.count++
	}
	return old, replaced, nil
}

func (idx *Index) Delete(key []byte) (Position, bool, error) {
	if err := checkKeySize(key); err != nil {
		return 0, false, err
	}

	pos, deleted, err := idx.deleteFrom(idx.rootID, key)
	if err != nil {
		return 0, false, err
	}
	if deleted {
		idx.count--
		if err := idx.compressRoot(); err != nil {
			return 0, false, err
		}
	}
	return pos, deleted, nil
}

func (idx *Index) Visit(fn func(key []byte, pos Position) bool) error {
	return idx.Ascend(fn)
}

func (idx *Index) LiveNodes() int {
	return idx.nodes.LiveNodes()
}

func (idx *Index) root() (*node, error) {
	return idx.nodeByID(idx.rootID)
}

func (idx *Index) nodeByID(id uint64) (*node, error) {
	return idx.nodes.Get(id)
}

func (idx *Index) allocNode() (uint64, *node, error) {
	return idx.nodes.Alloc()
}

func (idx *Index) freeNode(id uint64) error {
	return idx.nodes.Free(id)
}

func (idx *Index) key(pos Position) ([]byte, error) {
	key, ok := idx.records.Key(pos)
	if !ok {
		return nil, ErrMissingKey
	}
	if err := checkKeySize(key); err != nil {
		return nil, err
	}
	return key, nil
}
