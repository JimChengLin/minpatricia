package minpatricia

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"unsafe"
)

type memKeys map[Position][]byte

func (m memKeys) Key(pos Position) ([]byte, bool) {
	key, ok := m[pos]
	return key, ok
}

type nonZeroRootNodeStore struct {
	root  uint64
	nodes []*node
	live  int
}

func newNonZeroRootNodeStore(root uint64) *nonZeroRootNodeStore {
	nodes := make([]*node, root+1)
	nodes[root] = &node{}
	return &nonZeroRootNodeStore{
		root:  root,
		nodes: nodes,
		live:  1,
	}
}

func (s *nonZeroRootNodeStore) Root() uint64 {
	return s.root
}

func (s *nonZeroRootNodeStore) Get(id uint64) (*NodePage, error) {
	if id >= uint64(len(s.nodes)) || s.nodes[id] == nil {
		return nil, ErrCorruptLayout
	}
	return s.nodes[id], nil
}

func (s *nonZeroRootNodeStore) Alloc() (uint64, *NodePage, error) {
	id := uint64(len(s.nodes))
	if id&childTag != 0 {
		return 0, nil, ErrPositionTag
	}
	n := &node{}
	s.nodes = append(s.nodes, n)
	s.live++
	return id, n, nil
}

func (s *nonZeroRootNodeStore) Free(id uint64) error {
	if id == s.root || id >= uint64(len(s.nodes)) || s.nodes[id] == nil {
		return ErrCorruptLayout
	}
	s.nodes[id] = nil
	s.live--
	return nil
}

func (s *nonZeroRootNodeStore) LiveNodes() int {
	return s.live
}

func TestNodeLayout(t *testing.T) {
	if got := int(unsafe.Sizeof(node{})); got != NodeSize {
		t.Fatalf("sizeof(node{}) = %d, want %d", got, NodeSize)
	}
	if layoutBytes(MaxNodeReps) > NodeSize {
		t.Fatalf("layoutBytes(MaxNodeReps) = %d, want <= %d", layoutBytes(MaxNodeReps), NodeSize)
	}
	if layoutBytes(MaxNodeReps+1) <= NodeSize {
		t.Fatalf("layoutBytes(MaxNodeReps+1) = %d, want > %d", layoutBytes(MaxNodeReps+1), NodeSize)
	}
	if unsafe.Sizeof(route{}) != 4 {
		t.Fatalf("sizeof(route{}) = %d, want 4", unsafe.Sizeof(route{}))
	}
}

func TestDiffPrefixSemantics(t *testing.T) {
	diff, err := findDiffBit([]byte("A"), []byte("AA"))
	if err != nil {
		t.Fatal(err)
	}
	if diff != 9 {
		t.Fatalf("diff = %d, want 9", diff)
	}
	if getDiffBit([]byte("A"), diff) != 0 {
		t.Fatalf("GetDiffBit(A, 9) != 0")
	}
	if getDiffBit([]byte("AA"), diff) != 1 {
		t.Fatalf("GetDiffBit(AA, 9) != 1")
	}
}

func TestHeapNodeStoreReusesFreedNodeIDs(t *testing.T) {
	nodes := NewHeapNodeStore()
	id, n, err := nodes.Alloc()
	if err != nil {
		t.Fatal(err)
	}
	n.size = 7

	if err := nodes.Free(id); err != nil {
		t.Fatal(err)
	}
	if _, err := nodes.Get(id); err == nil {
		t.Fatalf("Get(%d) after Free succeeded", id)
	}

	nextID, next, err := nodes.Alloc()
	if err != nil {
		t.Fatal(err)
	}
	if nextID != id {
		t.Fatalf("reused id = %d, want %d", nextID, id)
	}
	if next == n {
		t.Fatalf("allocator reused freed node object")
	}
	if next.size != 0 {
		t.Fatalf("reused id node size = %d, want 0", next.size)
	}
	if nodes.LiveNodes() != 2 {
		t.Fatalf("LiveNodes = %d, want 2", nodes.LiveNodes())
	}
}

func TestHeapRecordStore(t *testing.T) {
	records := NewHeapRecordStore[string]()
	pos := records.Add([]byte("alpha"), "payload")
	if pos != 1 {
		t.Fatalf("pos = %d, want 1", pos)
	}

	key, ok := records.Key(pos)
	if !ok || string(key) != "alpha" {
		t.Fatalf("Key(%d) = (%q,%v), want (alpha,true)", pos, key, ok)
	}
	value, ok := records.Value(pos)
	if !ok || value != "payload" {
		t.Fatalf("Value(%d) = (%q,%v), want (payload,true)", pos, value, ok)
	}
	if records.Len() != 1 {
		t.Fatalf("Len = %d, want 1", records.Len())
	}
	nilKeyPos := records.Add(nil, "nil-key")
	key, ok = records.Key(nilKeyPos)
	if !ok || key == nil || len(key) != 0 {
		t.Fatalf("Key(%d) = (%v,%v), want empty key", nilKeyPos, key, ok)
	}
	if err := records.Free(pos); err != nil {
		t.Fatal(err)
	}
	if _, ok := records.Key(pos); ok {
		t.Fatalf("Key(%d) after Free succeeded", pos)
	}
	next := records.Add([]byte("bravo"), "second")
	if next != pos {
		t.Fatalf("reused pos = %d, want %d", next, pos)
	}
	value, ok = records.Value(next)
	if !ok || value != "second" {
		t.Fatalf("Value(%d) = (%q,%v), want (second,true)", next, value, ok)
	}
	if records.Len() != 2 {
		t.Fatalf("Len after reuse = %d, want 2", records.Len())
	}

	idxRecords := NewHeapRecordStore[string]()
	idxPos := idxRecords.Add([]byte("alpha"), "payload")
	idx := NewWithRecords(idxRecords)
	if _, replaced, err := idx.Put([]byte("alpha"), idxPos); err != nil || replaced {
		t.Fatalf("Put(alpha) replaced=%v err=%v", replaced, err)
	}
	got, found, err := idx.Get([]byte("alpha"))
	if err != nil || !found || got != idxPos {
		t.Fatalf("Get(alpha) = (%d,%v,%v), want (%d,true,nil)", got, found, err, idxPos)
	}
}

func TestNewHeapReturnsOwnedRecordStore(t *testing.T) {
	idx, records := NewHeap[string]()
	pos := records.Add([]byte("alpha"), "payload")
	if _, replaced, err := idx.Put([]byte("alpha"), pos); err != nil || replaced {
		t.Fatalf("Put(alpha) replaced=%v err=%v", replaced, err)
	}
	got, ok, err := idx.Get([]byte("alpha"))
	if err != nil || !ok || got != pos {
		t.Fatalf("Get(alpha) = (%d,%v,%v), want (%d,true,nil)", got, ok, err, pos)
	}
	value, ok := records.Value(got)
	if !ok || value != "payload" {
		t.Fatalf("Value(%d) = (%q,%v), want (payload,true)", got, value, ok)
	}
}

func TestDeleteUsesNodeStoreRoot(t *testing.T) {
	keys := memKeys{}
	idx := NewWithNodes(keys, newNonZeroRootNodeStore(3))

	for i, key := range []string{"alpha", "bravo", "charlie"} {
		pos := Position(i + 1)
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
	}

	deleted, ok, err := idx.Delete([]byte("bravo"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || deleted != 2 {
		t.Fatalf("Delete(bravo) = (%d,%v), want (2,true)", deleted, ok)
	}
	if _, ok, err := idx.Get([]byte("bravo")); err != nil || ok {
		t.Fatalf("Get(bravo) after delete ok=%v err=%v", ok, err)
	}
	if idx.Len() != 2 {
		t.Fatalf("Len = %d, want 2", idx.Len())
	}
}

func TestRootReadErrorIsReturned(t *testing.T) {
	idx := &Index{
		records: memKeys{},
		nodes:   NewHeapNodeStore(),
		rootID:  99,
	}

	if _, err := idx.root(); err != ErrCorruptLayout {
		t.Fatalf("root error = %v, want %v", err, ErrCorruptLayout)
	}
	if _, _, err := idx.Get([]byte("alpha")); err != ErrCorruptLayout {
		t.Fatalf("Get error = %v, want %v", err, ErrCorruptLayout)
	}
	if err := idx.Ascend(func(_ []byte, _ Position) bool {
		return true
	}); err != ErrCorruptLayout {
		t.Fatalf("Ascend error = %v, want %v", err, ErrCorruptLayout)
	}
}

func TestPutGetDeleteVisit(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)

	inputs := []string{"delta", "alpha", "charlie", "bravo", "echo", "a", "aa"}
	for i, key := range inputs {
		pos := Position(i + 1)
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
	}

	for i, key := range inputs {
		got, ok, err := idx.Get([]byte(key))
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if !ok || got != Position(i+1) {
			t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", key, got, ok, i+1)
		}
	}

	if _, ok, err := idx.Get([]byte("missing")); err != nil || ok {
		t.Fatalf("Get(missing) ok=%v err=%v", ok, err)
	}

	keys[99] = []byte("charlie")
	old, replaced, err := idx.Put([]byte("charlie"), 99)
	if err != nil {
		t.Fatal(err)
	}
	if !replaced || old != 3 {
		t.Fatalf("replace old=%d replaced=%v, want old=3 replaced=true", old, replaced)
	}

	deleted, ok, err := idx.Delete([]byte("bravo"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || deleted != 4 {
		t.Fatalf("Delete(bravo) = (%d, %v), want (4, true)", deleted, ok)
	}

	var visited []string
	if err := idx.Visit(func(key []byte, pos Position) bool {
		visited = append(visited, string(key))
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if !sort.StringsAreSorted(visited) {
		t.Fatalf("Visit order is not sorted: %v", visited)
	}
	if contains(visited, "bravo") {
		t.Fatalf("deleted key still visited: %v", visited)
	}
}

func TestIteratorAPI(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)

	inputs := []string{"delta", "alpha", "charlie", "bravo", "echo"}
	for i, key := range inputs {
		pos := Position(i + 1)
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
	}

	assertIterKeys(t, "Ascend", []string{"alpha", "bravo", "charlie", "delta", "echo"}, func(fn ItemIterator) error {
		return idx.Ascend(fn)
	})
	assertIterKeys(t, "AscendRange", []string{"bravo", "charlie", "delta"}, func(fn ItemIterator) error {
		return idx.AscendRange([]byte("bravo"), []byte("echo"), fn)
	})
	assertIterKeys(t, "AscendLessThan", []string{"alpha", "bravo", "charlie"}, func(fn ItemIterator) error {
		return idx.AscendLessThan([]byte("delta"), fn)
	})
	assertIterKeys(t, "AscendGreaterOrEqual", []string{"charlie", "delta", "echo"}, func(fn ItemIterator) error {
		return idx.AscendGreaterOrEqual([]byte("charlie"), fn)
	})
	assertIterKeys(t, "AscendGreaterOrEqual/missing-middle", []string{"charlie", "delta", "echo"}, func(fn ItemIterator) error {
		return idx.AscendGreaterOrEqual([]byte("caper"), fn)
	})
	assertIterKeys(t, "AscendGreaterOrEqual/past-end", nil, func(fn ItemIterator) error {
		return idx.AscendGreaterOrEqual([]byte("zulu"), fn)
	})
	assertIterKeys(t, "AscendRange/missing-lower", []string{"charlie", "delta"}, func(fn ItemIterator) error {
		return idx.AscendRange([]byte("caper"), []byte("echo"), fn)
	})
	assertIterKeys(t, "Descend", []string{"echo", "delta", "charlie", "bravo", "alpha"}, func(fn ItemIterator) error {
		return idx.Descend(fn)
	})
	assertIterKeys(t, "DescendRange", []string{"delta", "charlie"}, func(fn ItemIterator) error {
		return idx.DescendRange([]byte("delta"), []byte("bravo"), fn)
	})
	assertIterKeys(t, "DescendRange/missing-upper", []string{"charlie"}, func(fn ItemIterator) error {
		return idx.DescendRange([]byte("dazzle"), []byte("bravo"), fn)
	})
	assertIterKeys(t, "DescendLessOrEqual", []string{"delta", "charlie", "bravo", "alpha"}, func(fn ItemIterator) error {
		return idx.DescendLessOrEqual([]byte("delta"), fn)
	})
	assertIterKeys(t, "DescendLessOrEqual/missing-middle", []string{"bravo", "alpha"}, func(fn ItemIterator) error {
		return idx.DescendLessOrEqual([]byte("caper"), fn)
	})
	assertIterKeys(t, "DescendLessOrEqual/before-start", nil, func(fn ItemIterator) error {
		return idx.DescendLessOrEqual([]byte("aardvark"), fn)
	})
	assertIterKeys(t, "DescendLessOrEqual/past-end", []string{"echo", "delta", "charlie", "bravo", "alpha"}, func(fn ItemIterator) error {
		return idx.DescendLessOrEqual([]byte("zulu"), fn)
	})
	assertIterKeys(t, "DescendGreaterThan", []string{"echo", "delta", "charlie"}, func(fn ItemIterator) error {
		return idx.DescendGreaterThan([]byte("bravo"), fn)
	})
	assertIterKeys(t, "DescendGreaterThan/missing-lower", []string{"echo", "delta", "charlie"}, func(fn ItemIterator) error {
		return idx.DescendGreaterThan([]byte("caper"), fn)
	})

	var stopped []string
	if err := idx.Ascend(func(key []byte, _ Position) bool {
		stopped = append(stopped, string(key))
		return len(stopped) < 2
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(stopped), "[alpha bravo]"; got != want {
		t.Fatalf("early stop = %s, want %s", got, want)
	}
}

func TestOpenWithNodesUsesExistingNodes(t *testing.T) {
	keys := memKeys{}
	nodes := NewHeapNodeStore()
	idx := NewWithNodes(keys, nodes)

	for i, key := range []string{"alpha", "bravo", "charlie"} {
		pos := Position(i + 1)
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
	}

	reopened, err := OpenWithNodes(keys, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Len() != idx.Len() {
		t.Fatalf("Len = %d, want %d", reopened.Len(), idx.Len())
	}
	pos, ok, err := reopened.Get([]byte("bravo"))
	if err != nil || !ok || pos != 2 {
		t.Fatalf("Get(bravo) = (%d,%v,%v), want (2,true,nil)", pos, ok, err)
	}

}

func TestIteratorRangeMultiNode(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)

	rng := rand.New(rand.NewSource(7))
	seen := map[string]struct{}{}
	var sortedKeys []string
	for len(sortedKeys) < 2500 {
		key := fmt.Sprintf("k-%08x-%08x", rng.Uint32(), rng.Uint32())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		sortedKeys = append(sortedKeys, key)
		pos := Position(len(sortedKeys))
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
	}
	sort.Strings(sortedKeys)

	lo := sortedKeys[321]
	hi := sortedKeys[1987]
	assertIterKeys(t, "AscendRange/multi-node", sortedKeys[321:1987], func(fn ItemIterator) error {
		return idx.AscendRange([]byte(lo), []byte(hi), fn)
	})

	probes := []string{
		"k-00000000-00000000",
		sortedKeys[0],
		sortedKeys[100] + "x",
		sortedKeys[len(sortedKeys)-1],
		"k-ffffffff-ffffffff",
	}
	for i := 0; i < 100; i++ {
		probes = append(probes, fmt.Sprintf("k-%08x-%08x", rng.Uint32(), rng.Uint32()))
	}
	for _, probe := range probes {
		wantIdx := sort.SearchStrings(sortedKeys, probe)
		var got string
		if err := idx.AscendGreaterOrEqual([]byte(probe), func(key []byte, _ Position) bool {
			got = string(key)
			return false
		}); err != nil {
			t.Fatal(err)
		}
		if wantIdx == len(sortedKeys) {
			if got != "" {
				t.Fatalf("AscendGreaterOrEqual(%q) = %q, want no item", probe, got)
			}
			continue
		}
		if got != sortedKeys[wantIdx] {
			t.Fatalf("AscendGreaterOrEqual(%q) = %q, want %q", probe, got, sortedKeys[wantIdx])
		}
	}
	for _, probe := range probes {
		wantIdx := sort.Search(len(sortedKeys), func(i int) bool {
			return sortedKeys[i] > probe
		}) - 1
		var got string
		if err := idx.DescendLessOrEqual([]byte(probe), func(key []byte, _ Position) bool {
			got = string(key)
			return false
		}); err != nil {
			t.Fatal(err)
		}
		if wantIdx < 0 {
			if got != "" {
				t.Fatalf("DescendLessOrEqual(%q) = %q, want no item", probe, got)
			}
			continue
		}
		if got != sortedKeys[wantIdx] {
			t.Fatalf("DescendLessOrEqual(%q) = %q, want %q", probe, got, sortedKeys[wantIdx])
		}
	}

	wantDesc := make([]string, 0, 1987-322)
	for i := 1986; i > 321; i-- {
		wantDesc = append(wantDesc, sortedKeys[i])
	}
	assertIterKeys(t, "DescendRange/multi-node", wantDesc, func(fn ItemIterator) error {
		return idx.DescendRange([]byte(sortedKeys[1986]), []byte(sortedKeys[321]), fn)
	})
}

func TestCartesianRouteAgainstSortedMap(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)

	rng := rand.New(rand.NewSource(2))
	seen := map[string]struct{}{}
	for len(seen) < MaxNodeReps {
		key := []byte(fmt.Sprintf("key-%08x", rng.Uint32()))
		if _, ok := seen[string(key)]; ok {
			continue
		}
		seen[string(key)] = struct{}{}
		pos := Position(len(seen))
		keys[pos] = key
		if _, _, err := idx.Put(key, pos); err != nil {
			t.Fatal(err)
		}
	}

	for key := range seen {
		pos, ok, err := idx.Get([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if !ok || !bytes.Equal(keys[pos], []byte(key)) {
			t.Fatalf("Get(%q) = pos %d ok %v", key, pos, ok)
		}
	}

	keys[Position(MaxNodeReps+1)] = []byte("overflow")
	if _, _, err := idx.Put([]byte("overflow"), Position(MaxNodeReps+1)); err != nil {
		t.Fatalf("overflow insert: %v", err)
	}
	if idx.Len() != MaxNodeReps+1 {
		t.Fatalf("Len after split = %d, want %d", idx.Len(), MaxNodeReps+1)
	}
	if idx.LiveNodes() < 2 {
		t.Fatalf("expected split to allocate another node")
	}
}

func TestMultiNodeAgainstMap(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)
	expected := map[string]Position{}

	rng := rand.New(rand.NewSource(3))
	nextPos := Position(1)
	for len(expected) < 2500 {
		key := []byte(fmt.Sprintf("k-%08x-%08x", rng.Uint32(), rng.Uint32()))
		if _, exists := expected[string(key)]; exists {
			continue
		}
		pos := nextPos
		nextPos++
		keys[pos] = key
		if old, replaced, err := idx.Put(key, pos); err != nil || replaced || old != 0 {
			t.Fatalf("Put(%q) old=%d replaced=%v err=%v", key, old, replaced, err)
		}
		expected[string(key)] = pos
	}

	if idx.LiveNodes() < 2 {
		t.Fatalf("expected multi-node tree, live nodes=%d", idx.LiveNodes())
	}
	assertIndexMatchesMap(t, idx, keys, expected)

	var sortedKeys []string
	for key := range expected {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for i, key := range sortedKeys {
		if i%7 != 0 {
			continue
		}
		pos := nextPos
		nextPos++
		keys[pos] = []byte(key)
		old := expected[key]
		gotOld, replaced, err := idx.Put([]byte(key), pos)
		if err != nil {
			t.Fatal(err)
		}
		if !replaced || gotOld != old {
			t.Fatalf("replace %q old=%d replaced=%v, want old=%d replaced=true", key, gotOld, replaced, old)
		}
		expected[key] = pos
	}
	assertIndexMatchesMap(t, idx, keys, expected)

	for i, key := range sortedKeys {
		if i%3 != 0 {
			continue
		}
		want := expected[key]
		got, deleted, err := idx.Delete([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		if !deleted || got != want {
			t.Fatalf("Delete(%q) = (%d,%v), want (%d,true)", key, got, deleted, want)
		}
		delete(expected, key)
	}
	assertIndexMatchesMap(t, idx, keys, expected)

	for _, key := range sortedKeys {
		if _, ok := expected[key]; ok {
			continue
		}
		if _, ok, err := idx.Get([]byte(key)); err != nil || ok {
			t.Fatalf("deleted key %q Get ok=%v err=%v", key, ok, err)
		}
	}
}

func TestDeleteAllMaintainsRoutes(t *testing.T) {
	keys := memKeys{}
	idx := NewWithRecords(keys)
	expected := map[string]Position{}

	rng := rand.New(rand.NewSource(11))
	var deleteOrder []string
	nextPos := Position(1)
	for len(expected) < 700 {
		key := fmt.Sprintf("delete-%08x-%08x", rng.Uint32(), rng.Uint32())
		if _, exists := expected[key]; exists {
			continue
		}
		pos := nextPos
		nextPos++
		keys[pos] = []byte(key)
		if _, replaced, err := idx.Put([]byte(key), pos); err != nil || replaced {
			t.Fatalf("Put(%q) replaced=%v err=%v", key, replaced, err)
		}
		expected[key] = pos
		deleteOrder = append(deleteOrder, key)
	}

	rng.Shuffle(len(deleteOrder), func(i, j int) {
		deleteOrder[i], deleteOrder[j] = deleteOrder[j], deleteOrder[i]
	})

	for i, key := range deleteOrder {
		want := expected[key]
		got, deleted, err := idx.Delete([]byte(key))
		if err != nil || !deleted || got != want {
			t.Fatalf("Delete(%q) = (%d,%v,%v), want (%d,true,nil)", key, got, deleted, err, want)
		}
		delete(expected, key)
		if i%17 == 0 || len(expected) == 0 {
			assertIndexMatchesMap(t, idx, keys, expected)
		}
	}
}

func assertIndexMatchesMap(t *testing.T, idx *Index, keys memKeys, expected map[string]Position) {
	t.Helper()

	if idx.Len() != len(expected) {
		t.Fatalf("Len = %d, want %d", idx.Len(), len(expected))
	}

	for key, want := range expected {
		got, ok, err := idx.Get([]byte(key))
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if !ok || got != want {
			t.Fatalf("Get(%q) = (%d,%v), want (%d,true)", key, got, ok, want)
		}
		if !bytes.Equal(keys[got], []byte(key)) {
			t.Fatalf("position %d has key %q, want %q", got, keys[got], key)
		}
	}

	var visited []string
	if err := idx.Visit(func(key []byte, pos Position) bool {
		visited = append(visited, string(key))
		if expected[string(key)] != pos {
			t.Fatalf("Visit(%q) pos=%d, want %d", key, pos, expected[string(key)])
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}

	if len(visited) != len(expected) {
		t.Fatalf("Visit count = %d, want %d", len(visited), len(expected))
	}
	if !sort.StringsAreSorted(visited) {
		t.Fatalf("Visit order is not sorted")
	}
	for _, key := range visited {
		if _, ok := expected[key]; !ok {
			t.Fatalf("Visit returned unexpected key %q", key)
		}
	}
	assertIndexRoutesValid(t, idx)
}

func assertIterKeys(t *testing.T, name string, want []string, run func(ItemIterator) error) {
	t.Helper()

	var got []string
	err := run(func(key []byte, _ Position) bool {
		got = append(got, string(key))
		return true
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assertIndexRoutesValid(t *testing.T, idx *Index) {
	t.Helper()

	root, err := idx.root()
	if err != nil {
		t.Fatal(err)
	}
	assertNodeRoutesValid(t, idx, root)
}

func assertNodeRoutesValid(t *testing.T, idx *Index, n *node) {
	t.Helper()

	size := int(n.size)
	if size > 1 {
		var gotBuf [MaxNodeReps - 1]uint16
		got := gotBuf[:size-1]
		if err := n.routeDiffs(got); err != nil {
			t.Fatal(err)
		}
		want, err := idx.buildDiffs(n.reps[:size])
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("route diff[%d] = %d, want %d", i, got[i], want[i])
			}
		}
	}

	for i := 0; i < size; i++ {
		r := n.reps[i]
		if !r.isChild() {
			continue
		}
		child, err := idx.nodeByID(r.childID())
		if err != nil {
			t.Fatal(err)
		}
		assertNodeRoutesValid(t, idx, child)
	}
}
