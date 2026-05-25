package minpatricia

type rebuildScratch struct {
	diffs      []uint16
	left       []int
	right      []int
	cartStack  []int
	routeStack []routeFrame
}

func (idx *Index) rebuildNode(n *node) error {
	size := int(n.size)
	if size == 0 {
		n.firstPos = 0
		n.lastPos = 0
		clearRoutes(n)
		return nil
	}

	firstPos, err := idx.minPos(n.reps[0])
	if err != nil {
		return err
	}
	lastPos, err := idx.maxPos(n.reps[size-1])
	if err != nil {
		return err
	}
	n.firstPos = firstPos
	n.lastPos = lastPos

	if size == 1 {
		clearRoutes(n)
		return nil
	}

	diffs, err := idx.buildDiffs(n.reps[:size])
	if err != nil {
		return err
	}
	diffCount := len(diffs)

	left, right, root := idx.buildCartesian(diffs)
	idx.writePreorderRoutes(n.routes[:diffCount], diffs, left, right, root, size)
	return nil
}

func (idx *Index) rebuildNodeWithDiffs(n *node, diffs []uint16) error {
	size := int(n.size)
	if len(diffs) != size-1 {
		return ErrCorruptLayout
	}
	if size == 0 {
		n.firstPos = 0
		n.lastPos = 0
		clearRoutes(n)
		return nil
	}

	firstPos, err := idx.minPos(n.reps[0])
	if err != nil {
		return err
	}
	lastPos, err := idx.maxPos(n.reps[size-1])
	if err != nil {
		return err
	}
	n.firstPos = firstPos
	n.lastPos = lastPos

	if size == 1 {
		clearRoutes(n)
		return nil
	}

	left, right, root := idx.buildCartesian(diffs)
	idx.writePreorderRoutes(n.routes[:len(diffs)], diffs, left, right, root, size)
	return nil
}

func (idx *Index) buildDiffs(reps []rep) ([]uint16, error) {
	diffCount := len(reps) - 1
	if cap(idx.scratch.diffs) < diffCount {
		idx.scratch.diffs = make([]uint16, diffCount)
	}
	diffs := idx.scratch.diffs[:diffCount]

	for i := 0; i < diffCount; i++ {
		diff, err := idx.diffBetweenReps(reps[i], reps[i+1])
		if err != nil {
			return nil, err
		}
		diffs[i] = diff
	}
	return diffs, nil
}

func (idx *Index) diffBetweenReps(left, right rep) (uint16, error) {
	leftPos, err := idx.maxPos(left)
	if err != nil {
		return 0, err
	}
	rightPos, err := idx.minPos(right)
	if err != nil {
		return 0, err
	}

	leftKey, err := idx.key(leftPos)
	if err != nil {
		return 0, err
	}
	rightKey, err := idx.key(rightPos)
	if err != nil {
		return 0, err
	}

	cmp, diff, err := compareAndDiffBit(leftKey, rightKey)
	if err != nil {
		if err == ErrEqualKeys {
			return 0, ErrDuplicateKey
		}
		return 0, err
	}
	if cmp > 0 {
		return 0, ErrUnsortedKeys
	}
	return diff, nil
}

func clearRoutes(n *node) {
	for i := range n.routes {
		n.routes[i] = route{}
	}
}

func (idx *Index) minPos(r rep) (Position, error) {
	if !r.isChild() {
		return r.position(), nil
	}
	child, err := idx.nodeByID(r.childID())
	if err != nil {
		return 0, err
	}
	if child.size == 0 {
		return 0, ErrCorruptLayout
	}
	return child.firstPos, nil
}

func (idx *Index) maxPos(r rep) (Position, error) {
	if !r.isChild() {
		return r.position(), nil
	}
	child, err := idx.nodeByID(r.childID())
	if err != nil {
		return 0, err
	}
	if child.size == 0 {
		return 0, ErrCorruptLayout
	}
	return child.lastPos, nil
}
