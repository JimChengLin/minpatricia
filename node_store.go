package minpatricia

// NodeStore maps stable node IDs to fixed-size node pages.
//
// A file-backed implementation can use page indexes as IDs and translate them
// as mmap_base + id * NodeSize. Since child reps store IDs instead of Go
// pointers, the trie structure can be remapped without rewriting child links.
type NodeStore interface {
	Root() uint64
	Get(id uint64) (*NodePage, error)
	Alloc() (uint64, *NodePage, error)
	Free(id uint64) error
	LiveNodes() int
}

type HeapNodeStore struct {
	nodes []*node
	free  []uint64
	live  int
}

func NewHeapNodeStore() *HeapNodeStore {
	return &HeapNodeStore{
		nodes: []*node{{}},
		live:  1,
	}
}

func (a *HeapNodeStore) Root() uint64 {
	return 0
}

func (a *HeapNodeStore) Get(id uint64) (*NodePage, error) {
	if id >= uint64(len(a.nodes)) || a.nodes[id] == nil {
		return nil, ErrCorruptLayout
	}
	return a.nodes[id], nil
}

func (a *HeapNodeStore) Alloc() (uint64, *NodePage, error) {
	if len(a.free) != 0 {
		last := len(a.free) - 1
		id := a.free[last]
		a.free[last] = 0
		a.free = a.free[:last]
		n := &node{}
		a.nodes[id] = n
		a.live++
		return id, n, nil
	}

	id := uint64(len(a.nodes))
	if id&childTag != 0 {
		return 0, nil, ErrPositionTag
	}
	n := &node{}
	a.nodes = append(a.nodes, n)
	a.live++
	return id, n, nil
}

func (a *HeapNodeStore) Free(id uint64) error {
	if id == a.Root() {
		return ErrCorruptLayout
	}
	if id >= uint64(len(a.nodes)) || a.nodes[id] == nil {
		return ErrCorruptLayout
	}
	a.nodes[id] = nil
	a.free = append(a.free, id)
	a.live--
	return nil
}

func (a *HeapNodeStore) LiveNodes() int {
	return a.live
}
