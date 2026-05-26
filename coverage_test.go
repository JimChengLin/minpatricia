package minpatricia

import (
	"fmt"
	"testing"
)

func TestRecordStoreFuncAndHeapRecordAccessors(t *testing.T) {
	calls := 0
	records := RecordStoreFunc(func(pos Position) ([]byte, bool) {
		calls++
		if pos != 7 {
			return nil, false
		}
		return []byte("seven"), true
	})
	key, ok := records.Key(7)
	if !ok || string(key) != "seven" || calls != 1 {
		t.Fatalf("RecordStoreFunc.Key = (%q,%v), calls=%d, want (seven,true), calls=1", key, ok, calls)
	}
	if key, ok := records.Key(8); ok || key != nil {
		t.Fatalf("RecordStoreFunc.Key missing = (%q,%v), want (nil,false)", key, ok)
	}

	heap := NewHeapRecordStore[int]()
	if rec, ok := heap.Record(0); ok || rec.Key != nil || rec.Value != 0 {
		t.Fatalf("Record(0) = (%v,%v), want zero,false", rec, ok)
	}
	pos := heap.Add([]byte("alpha"), 42)
	rec, ok := heap.Record(pos)
	if !ok || string(rec.Key) != "alpha" || rec.Value != 42 {
		t.Fatalf("Record(%d) = (%v,%v), want alpha/42,true", pos, rec, ok)
	}
	if err := heap.Free(pos); err != nil {
		t.Fatal(err)
	}
	if rec, ok := heap.Record(pos); ok || rec.Key != nil || rec.Value != 0 {
		t.Fatalf("Record(%d) after Free = (%v,%v), want zero,false", pos, rec, ok)
	}
	if value, ok := heap.Value(pos); ok || value != 0 {
		t.Fatalf("Value(%d) after Free = (%d,%v), want 0,false", pos, value, ok)
	}
}

func TestConstructorsAndPublicErrorPaths(t *testing.T) {
	if _, err := OpenWithNodes(nil, NewHeapNodeStore()); err != ErrNilRecordStore {
		t.Fatalf("OpenWithNodes nil records err = %v, want %v", err, ErrNilRecordStore)
	}
	if _, err := OpenWithNodes(memKeys{}, nil); err != ErrNilNodeStore {
		t.Fatalf("OpenWithNodes nil nodes err = %v, want %v", err, ErrNilNodeStore)
	}
	assertPanicsWith(t, ErrNilRecordStore, func() {
		NewWithRecords(nil)
	})
	assertPanicsWith(t, ErrNilNodeStore, func() {
		NewWithNodes(memKeys{}, nil)
	})

	idx := NewWithRecords(memKeys{})
	tooLarge := make([]byte, MaxKeySize+1)
	if _, _, err := idx.Get(tooLarge); err != ErrKeyTooLarge {
		t.Fatalf("Get too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if _, _, err := idx.Put(tooLarge, 1); err != ErrKeyTooLarge {
		t.Fatalf("Put too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if _, _, err := idx.Delete(tooLarge); err != ErrKeyTooLarge {
		t.Fatalf("Delete too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.AscendRange(tooLarge, nil, nil); err != ErrKeyTooLarge {
		t.Fatalf("AscendRange lower too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.AscendRange(nil, tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("AscendRange upper too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.DescendRange(tooLarge, nil, nil); err != ErrKeyTooLarge {
		t.Fatalf("DescendRange upper too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.DescendRange(nil, tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("DescendRange lower too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.AscendLessThan(tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("AscendLessThan too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.AscendGreaterOrEqual(tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("AscendGreaterOrEqual too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.DescendLessOrEqual(tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("DescendLessOrEqual too-large err = %v, want %v", err, ErrKeyTooLarge)
	}
	if err := idx.DescendGreaterThan(tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("DescendGreaterThan too-large err = %v, want %v", err, ErrKeyTooLarge)
	}

	keys := memKeys{
		1:                  []byte("one"),
		Position(childTag): []byte("tagged"),
	}
	idx = NewWithRecords(keys)
	if _, _, err := idx.Put([]byte("missing"), 99); err != ErrMissingKey {
		t.Fatalf("Put missing pos err = %v, want %v", err, ErrMissingKey)
	}
	if _, _, err := idx.Put([]byte("mismatch"), 1); err != ErrPositionKey {
		t.Fatalf("Put mismatched key err = %v, want %v", err, ErrPositionKey)
	}
	if _, _, err := idx.Put([]byte("tagged"), Position(childTag)); err != ErrPositionTag {
		t.Fatalf("Put tagged position err = %v, want %v", err, ErrPositionTag)
	}
	if _, err := makeRecordRep(Position(childTag)); err != ErrPositionTag {
		t.Fatalf("makeRecordRep tagged err = %v, want %v", err, ErrPositionTag)
	}
	if _, err := makeChildRep(childTag); err != ErrPositionTag {
		t.Fatalf("makeChildRep tagged err = %v, want %v", err, ErrPositionTag)
	}
}

func assertPanicsWith(t *testing.T, want error, fn func()) {
	t.Helper()

	defer func() {
		got := recover()
		if got != want {
			t.Fatalf("panic = %v, want %v", got, want)
		}
	}()
	fn()
}

func TestDiffAndRouteErrorPaths(t *testing.T) {
	tooLarge := make([]byte, MaxKeySize+1)
	if _, _, err := compareAndDiffBit(tooLarge, nil); err != ErrKeyTooLarge {
		t.Fatalf("compareAndDiffBit large left err = %v, want %v", err, ErrKeyTooLarge)
	}
	if _, _, err := compareAndDiffBit(nil, tooLarge); err != ErrKeyTooLarge {
		t.Fatalf("compareAndDiffBit large right err = %v, want %v", err, ErrKeyTooLarge)
	}
	if got := getDiffBit(nil, 0); got != 0 {
		t.Fatalf("getDiffBit(nil,0) = %d, want 0", got)
	}
	if got := getDiffBit([]byte{0x80}, 1); got != 1 {
		t.Fatalf("getDiffBit(0x80,1) = %d, want 1", got)
	}
	for _, tc := range []struct {
		diff      uint16
		leftCount uint16
		key       []byte
		wantBit   uint8
	}{
		{diff: 0, leftCount: 1, key: nil, wantBit: 0},
		{diff: 0, leftCount: 1, key: []byte{0}, wantBit: 1},
		{diff: 1, leftCount: 2, key: []byte{0x80}, wantBit: 1},
		{diff: 8, leftCount: 3, key: []byte{1}, wantBit: 1},
		{diff: maxDiff, leftCount: MaxNodeReps, key: nil, wantBit: 0},
	} {
		r := makeRoute(tc.diff, tc.leftCount)
		if got := r.diff(); got != tc.diff {
			t.Fatalf("route.diff(%d) = %d", tc.diff, got)
		}
		if got := r.leftCount(); got != tc.leftCount {
			t.Fatalf("route.leftCount(%d) = %d, want %d", tc.diff, got, tc.leftCount)
		}
		if got := r.bit(tc.key); got != tc.wantBit {
			t.Fatalf("route.bit(%x,%d) = %d, want %d", tc.key, tc.diff, got, tc.wantBit)
		}
	}

	var empty node
	if leaf, ok := empty.lookupRouteOnly([]byte("x")); ok || leaf != 0 {
		t.Fatalf("empty lookup = (%d,%v), want (0,false)", leaf, ok)
	}
	if slot, ok, err := empty.insertSlotAbovePath([]byte("x"), 1, 0); err != nil || ok || slot != 0 {
		t.Fatalf("empty insertSlotAbovePath = (%d,%v,%v), want (0,false,nil)", slot, ok, err)
	}
	if err := empty.insertRoute(0, []byte("x"), 1); err != ErrCorruptLayout {
		t.Fatalf("empty insertRoute err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := empty.deleteRoute(0); err != ErrCorruptLayout {
		t.Fatalf("empty deleteRoute err = %v, want %v", err, ErrCorruptLayout)
	}

	n := &node{size: 1}
	if _, _, err := n.insertSlotAbovePath([]byte("x"), 1, 1); err != ErrCorruptLayout {
		t.Fatalf("insertSlotAbovePath bad leaf err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := n.insertRoute(2, []byte{0}, 1); err != ErrCorruptLayout {
		t.Fatalf("insertRoute bad slot err = %v, want %v", err, ErrCorruptLayout)
	}

	corrupt := &node{size: 2}
	if err := corrupt.routeDiffs(nil); err != ErrCorruptLayout {
		t.Fatalf("routeDiffs len mismatch err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := corrupt.routeDiffs(make([]uint16, 1)); err != ErrCorruptLayout {
		t.Fatalf("routeDiffs zero leftCount err = %v, want %v", err, ErrCorruptLayout)
	}
	if _, _, err := corrupt.insertSlotAbovePath([]byte("x"), 1, 0); err != ErrCorruptLayout {
		t.Fatalf("insertSlotAbovePath corrupt err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := corrupt.insertRoute(0, []byte("x"), 1); err != ErrCorruptLayout {
		t.Fatalf("insertRoute corrupt err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := corrupt.deleteRoute(0); err != ErrCorruptLayout {
		t.Fatalf("deleteRoute corrupt err = %v, want %v", err, ErrCorruptLayout)
	}
	if corrupt.isDirectRoutePair(-1) {
		t.Fatalf("isDirectRoutePair(-1) = true, want false")
	}
	if corrupt.isDirectRoutePair(1) {
		t.Fatalf("isDirectRoutePair(1) = true, want false")
	}
}

func TestInternalBuildAndNodePaths(t *testing.T) {
	keys := memKeys{
		1: []byte("alpha"),
		2: []byte("bravo"),
	}
	idx := NewWithRecords(keys)
	n := &node{size: 2, reps: [MaxNodeReps]rep{rep(1), rep(2)}}
	if err := idx.rebuildNodeWithDiffs(n, nil); err != ErrCorruptLayout {
		t.Fatalf("rebuildNodeWithDiffs len mismatch err = %v, want %v", err, ErrCorruptLayout)
	}

	empty := &node{
		firstPos: 1,
		lastPos:  2,
		routes:   [MaxNodeReps - 1]route{makeRoute(7, 1)},
	}
	if err := idx.rebuildNodeWithDiffs(empty, nil); err != ErrCorruptLayout {
		t.Fatalf("rebuildNodeWithDiffs empty err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := idx.rebuildNode(empty); err != nil {
		t.Fatal(err)
	}
	if empty.firstPos != 0 || empty.lastPos != 0 {
		t.Fatalf("empty rebuilt bounds = (%d,%d), want (0,0)", empty.firstPos, empty.lastPos)
	}

	one := &node{size: 1, reps: [MaxNodeReps]rep{rep(1)}}
	if err := idx.rebuildNodeWithDiffs(one, nil); err != nil {
		t.Fatal(err)
	}
	if one.firstPos != 1 || one.lastPos != 1 {
		t.Fatalf("single rebuilt bounds = (%d,%d), want (1,1)", one.firstPos, one.lastPos)
	}

	missing := NewWithRecords(memKeys{})
	if err := missing.rebuildNode(&node{size: 2, reps: [MaxNodeReps]rep{rep(1), rep(2)}}); err != ErrMissingKey {
		t.Fatalf("rebuildNode missing key err = %v, want %v", err, ErrMissingKey)
	}

	dup := NewWithRecords(memKeys{1: []byte("same"), 2: []byte("same")})
	if _, err := dup.diffBetweenReps(rep(1), rep(2)); err != ErrDuplicateKey {
		t.Fatalf("diffBetweenReps duplicate err = %v, want %v", err, ErrDuplicateKey)
	}
	unsorted := NewWithRecords(memKeys{1: []byte("z"), 2: []byte("a")})
	if _, err := unsorted.diffBetweenReps(rep(1), rep(2)); err != ErrUnsortedKeys {
		t.Fatalf("diffBetweenReps unsorted err = %v, want %v", err, ErrUnsortedKeys)
	}

	nodes := NewHeapNodeStore()
	childID, child, err := nodes.Alloc()
	if err != nil {
		t.Fatal(err)
	}
	idx = NewWithNodes(keys, nodes)
	childRep, err := makeChildRep(childID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.minPos(childRep); err != ErrCorruptLayout {
		t.Fatalf("minPos empty child err = %v, want %v", err, ErrCorruptLayout)
	}
	if _, err := idx.maxPos(childRep); err != ErrCorruptLayout {
		t.Fatalf("maxPos empty child err = %v, want %v", err, ErrCorruptLayout)
	}

	child.size = 2
	child.reps[0] = rep(1)
	child.reps[1] = rep(2)
	if err := idx.rebuildNode(child); err != nil {
		t.Fatal(err)
	}
	root, err := idx.root()
	if err != nil {
		t.Fatal(err)
	}
	root.size = 1
	root.reps[0] = childRep
	root.firstPos = child.firstPos
	root.lastPos = child.lastPos
	if err := idx.compressRoot(); err != nil {
		t.Fatal(err)
	}
	if root.size != child.size || root.firstPos != 1 || root.lastPos != 2 || nodes.LiveNodes() != 1 {
		t.Fatalf("compressRoot root size=%d bounds=(%d,%d) live=%d, want size=2 bounds=(1,2) live=1", root.size, root.firstPos, root.lastPos, nodes.LiveNodes())
	}

	if err := idx.writeNodeWithDiffs(idx.rootID, []rep{rep(1), rep(2)}, nil); err != ErrCorruptLayout {
		t.Fatalf("writeNodeWithDiffs bad diffs err = %v, want %v", err, ErrCorruptLayout)
	}
	if err := idx.writeNode(99, []rep{rep(1)}); err != ErrCorruptLayout {
		t.Fatalf("writeNode missing node err = %v, want %v", err, ErrCorruptLayout)
	}
}

func TestSplitAndCountInternalPaths(t *testing.T) {
	keys := memKeys{}
	reps := make([]rep, MaxNodeReps+1)
	for i := range reps {
		pos := Position(i + 1)
		keys[pos] = []byte(fmt.Sprintf("split-%04d", i))
		reps[i] = rep(pos)
	}

	idx := NewWithRecords(keys)
	if err := idx.splitAndWriteNode(idx.rootID, reps); err != nil {
		t.Fatal(err)
	}
	if idx.LiveNodes() < 2 {
		t.Fatalf("LiveNodes after split = %d, want >= 2", idx.LiveNodes())
	}
	root, err := idx.root()
	if err != nil {
		t.Fatal(err)
	}
	count, err := idx.countRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != len(reps) {
		t.Fatalf("countRecords = %d, want %d", count, len(reps))
	}
	assertNodeRoutesValid(t, idx, root)

	fullReps := make([]rep, MaxNodeReps+1)
	for i := range fullReps {
		fullReps[i] = rep(Position(i + 1))
	}
	fullIdx := NewWithRecords(keys)
	full := &node{size: MaxNodeReps}
	copy(full.reps[:], fullReps[:MaxNodeReps])
	if err := fullIdx.rebuildNode(full); err != nil {
		t.Fatal(err)
	}
	if err := fullIdx.insertFullAt(fullIdx.rootID, full, MaxNodeReps, fullReps[MaxNodeReps]); err != nil {
		t.Fatal(err)
	}
	if fullIdx.LiveNodes() < 2 {
		t.Fatalf("LiveNodes after insertFullAt = %d, want >= 2", fullIdx.LiveNodes())
	}

	root.reps[0] = rep(childTag | 99)
	if _, err := idx.countRecords(root); err != ErrCorruptLayout {
		t.Fatalf("countRecords missing child err = %v, want %v", err, ErrCorruptLayout)
	}
}

func TestEmptyIteratorsAndStoreErrorPaths(t *testing.T) {
	idx := NewWithRecords(memKeys{})
	mustNotVisit := func(key []byte, pos Position) bool {
		t.Fatalf("unexpected visit key=%q pos=%d", key, pos)
		return false
	}
	if err := idx.Ascend(mustNotVisit); err != nil {
		t.Fatalf("Ascend empty: %v", err)
	}
	if err := idx.Descend(mustNotVisit); err != nil {
		t.Fatalf("Descend empty: %v", err)
	}
	if err := idx.AscendGreaterOrEqual([]byte("a"), mustNotVisit); err != nil {
		t.Fatalf("AscendGreaterOrEqual empty: %v", err)
	}
	if err := idx.DescendLessOrEqual([]byte("a"), mustNotVisit); err != nil {
		t.Fatalf("DescendLessOrEqual empty: %v", err)
	}

	heap := NewHeapRecordStore[int]()
	if err := heap.Free(0); err != ErrMissingKey {
		t.Fatalf("HeapRecordStore.Free(0) = %v, want %v", err, ErrMissingKey)
	}
	if key, ok := heap.Key(1); ok || key != nil {
		t.Fatalf("HeapRecordStore.Key missing = (%q,%v), want (nil,false)", key, ok)
	}
	if value, ok := heap.Value(1); ok || value != 0 {
		t.Fatalf("HeapRecordStore.Value missing = (%d,%v), want (0,false)", value, ok)
	}

	nodes := NewHeapNodeStore()
	if err := nodes.Free(99); err != ErrCorruptLayout {
		t.Fatalf("HeapNodeStore.Free missing = %v, want %v", err, ErrCorruptLayout)
	}
	if got := layoutBytes(0); got != 24 {
		t.Fatalf("layoutBytes(0) = %d, want 24", got)
	}
}

func TestCorruptOpenAndChildDeletePaths(t *testing.T) {
	nodes := newNonZeroRootNodeStore(1)
	nodes.nodes[1] = nil
	if _, err := OpenWithNodes(memKeys{}, nodes); err != ErrCorruptLayout {
		t.Fatalf("OpenWithNodes corrupt root err = %v, want %v", err, ErrCorruptLayout)
	}

	heapNodes := NewHeapNodeStore()
	childID, _, err := heapNodes.Alloc()
	if err != nil {
		t.Fatal(err)
	}
	idx := NewWithNodes(memKeys{1: []byte("alpha")}, heapNodes)
	parent := &node{size: 1}
	if _, deleted, err := idx.deleteFromChild(parent, 0, childID, []byte("alpha")); err != ErrCorruptLayout || deleted {
		t.Fatalf("deleteFromChild empty child = deleted %v err %v, want false/%v", deleted, err, ErrCorruptLayout)
	}
}

func TestIterPathOverflowAndCurrentRecordErrors(t *testing.T) {
	var path iterPath
	for i := 0; i < iterStackDepth+3; i++ {
		path.push(uint64(i+1), i)
	}
	frames := path.framesForInsert()
	if len(frames) != iterStackDepth+3 || frames[len(frames)-1].id != uint64(iterStackDepth+3) {
		t.Fatalf("overflow frames len=%d last=%v, want len=%d last id=%d", len(frames), frames[len(frames)-1], iterStackDepth+3, iterStackDepth+3)
	}
	if got := path.at(iterStackDepth + 1); got.id != uint64(iterStackDepth+2) {
		t.Fatalf("overflow at id = %d, want %d", got.id, iterStackDepth+2)
	}
	path.truncate(iterStackDepth + 1)
	if path.len != iterStackDepth+1 || len(path.overflow) != 1 {
		t.Fatalf("truncate overflow len=%d overflow=%d, want %d/1", path.len, len(path.overflow), iterStackDepth+1)
	}
	path.truncate(3)
	if path.len != 3 || len(path.overflow) != 0 {
		t.Fatalf("truncate stack len=%d overflow=%d, want 3/0", path.len, len(path.overflow))
	}

	idx := NewWithRecords(memKeys{})
	path.reset()
	if _, _, err := idx.currentRecord(&path); err != ErrCorruptLayout {
		t.Fatalf("currentRecord empty path err = %v, want %v", err, ErrCorruptLayout)
	}
	if ok, err := idx.positionAtOrAfter(&path, -1, 0); err != ErrCorruptLayout || ok {
		t.Fatalf("positionAtOrAfter bad target = (%v,%v), want false/%v", ok, err, ErrCorruptLayout)
	}
	if ok, err := idx.positionAtOrBefore(&path, -1, 0); err != ErrCorruptLayout || ok {
		t.Fatalf("positionAtOrBefore bad target = (%v,%v), want false/%v", ok, err, ErrCorruptLayout)
	}
	path.push(99, 0)
	if _, _, err := idx.currentRecord(&path); err != ErrCorruptLayout {
		t.Fatalf("currentRecord missing node err = %v, want %v", err, ErrCorruptLayout)
	}
	path.reset()
	path.push(idx.rootID, 0)
	if _, _, err := idx.currentRecord(&path); err != ErrCorruptLayout {
		t.Fatalf("currentRecord empty root err = %v, want %v", err, ErrCorruptLayout)
	}
	root, err := idx.root()
	if err != nil {
		t.Fatal(err)
	}
	root.size = 1
	root.reps[0] = rep(childTag)
	if _, _, err := idx.currentRecord(&path); err != ErrCorruptLayout {
		t.Fatalf("currentRecord child rep err = %v, want %v", err, ErrCorruptLayout)
	}
}
