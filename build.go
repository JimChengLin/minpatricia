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

func (idx *Index) buildCartesian(diffs []uint16) ([]int, []int, int) {
	n := len(diffs)
	if cap(idx.scratch.left) < n {
		idx.scratch.left = make([]int, n)
	}
	if cap(idx.scratch.right) < n {
		idx.scratch.right = make([]int, n)
	}
	left := idx.scratch.left[:n]
	right := idx.scratch.right[:n]
	for i := 0; i < n; i++ {
		left[i] = -1
		right[i] = -1
	}

	if cap(idx.scratch.cartStack) < n {
		idx.scratch.cartStack = make([]int, 0, n)
	}
	stack := idx.scratch.cartStack[:0]
	for i := 0; i < n; i++ {
		last := -1
		for len(stack) > 0 && diffs[i] <= diffs[stack[len(stack)-1]] {
			last = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
		if last != -1 {
			left[i] = last
		}
		if len(stack) > 0 {
			right[stack[len(stack)-1]] = i
		}
		stack = append(stack, i)
	}
	root := stack[0]
	idx.scratch.cartStack = stack[:0]
	return left, right, root
}

type routeFrame struct {
	node  int
	leafL int
	leafR int
}

func (idx *Index) writePreorderRoutes(out []route, diffs []uint16, left, right []int, root int, size int) {
	if cap(idx.scratch.routeStack) < size {
		idx.scratch.routeStack = make([]routeFrame, 0, size)
	}
	stack := idx.scratch.routeStack[:0]
	stack = append(stack, routeFrame{node: root, leafL: 0, leafR: size})
	outIdx := 0

	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		i := frame.node
		out[outIdx] = route{
			diff:      diffs[i],
			leftCount: uint16(i - frame.leafL + 1),
		}
		outIdx++

		if right[i] != -1 {
			stack = append(stack, routeFrame{
				node:  right[i],
				leafL: i + 1,
				leafR: frame.leafR,
			})
		}
		if left[i] != -1 {
			stack = append(stack, routeFrame{
				node:  left[i],
				leafL: frame.leafL,
				leafR: i + 1,
			})
		}
	}
	idx.scratch.routeStack = stack[:0]
}
