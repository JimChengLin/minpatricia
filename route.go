package minpatricia

type routeFrame struct {
	node  int
	leafL int
	leafR int
}

const (
	routeByteMask        = 1<<13 - 1
	routeBitOpOffset     = 13
	routeBitOpMask       = 0xf
	routeTermBitOp       = 8
	routeLeftCountOffset = 17
)

// route stores the decoded diff operation beside leftCount so lookup avoids
// dividing each route diff by 9 while walking the node.
func makeRoute(diff uint16, leftCount uint16) route {
	d := uint32(diff)
	byteIdx := d / 9
	bitOp := uint32(routeTermBitOp)
	// bitOp is the shift count for byte bits, or routeTermBitOp for terminator diffs.
	if bitIdx := d % 9; bitIdx != 0 {
		bitOp = 8 - bitIdx
	}
	return route{
		bits: byteIdx | bitOp<<routeBitOpOffset | uint32(leftCount)<<routeLeftCountOffset,
	}
}

func (r route) diff() uint16 {
	byteIdx := r.bits & routeByteMask
	bitOp := (r.bits >> routeBitOpOffset) & routeBitOpMask
	if bitOp == routeTermBitOp {
		return uint16(byteIdx * 9)
	}
	return uint16(byteIdx*9 + 8 - bitOp)
}

func (r route) leftCount() uint16 {
	return uint16(r.bits >> routeLeftCountOffset)
}

func (r *route) incLeftCount() {
	r.bits += 1 << routeLeftCountOffset
}

func (r *route) decLeftCount() {
	r.bits -= 1 << routeLeftCountOffset
}

func (r route) bit(key []byte) uint8 {
	byteIdx := int(r.bits & routeByteMask)
	if byteIdx >= len(key) {
		return 0
	}
	bitOp := uint8(r.bits>>routeBitOpOffset) & routeBitOpMask
	return bitOp>>3 | ((key[byteIdx] >> (bitOp & 7)) & 1)
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
		out[outIdx] = makeRoute(diffs[i], uint16(i-frame.leafL+1))
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

func (n *node) lookup(key []byte) (int, bool) {
	return n.lookupRouteOnly(key)
}

func (n *node) lookupRouteOnly(key []byte) (int, bool) {
	size := int(n.size)
	if size == 0 {
		return 0, false
	}

	routeIdx := 0
	leafBase := 0
	leafCount := size

	for leafCount > 1 {
		r := n.routes[routeIdx]
		leftCount := int(r.leftCount())
		if r.bit(key) == 0 {
			routeIdx++
			leafCount = leftCount
		} else {
			routeIdx += leftCount
			leafBase += leftCount
			leafCount -= leftCount
		}
	}
	return leafBase, true
}

func (n *node) routeDiffs(out []uint16) error {
	size := int(n.size)
	if len(out) != size-1 {
		return ErrCorruptLayout
	}
	if size <= 1 {
		return nil
	}

	var stackBuf [MaxNodeReps]routeFrame
	stack := stackBuf[:1]
	stack[0] = routeFrame{node: 0, leafL: 0, leafR: size}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if frame.node < 0 || frame.node >= size-1 {
			return ErrCorruptLayout
		}
		r := n.routes[frame.node]
		leftCount := int(r.leftCount())
		diffIdx := frame.leafL + leftCount - 1
		if leftCount <= 0 || diffIdx < frame.leafL || diffIdx >= frame.leafR-1 {
			return ErrCorruptLayout
		}
		out[diffIdx] = r.diff()

		rightIdx := frame.node + leftCount
		if diffIdx+1 < frame.leafR-1 {
			stack = append(stack, routeFrame{
				node:  rightIdx,
				leafL: diffIdx + 1,
				leafR: frame.leafR,
			})
		}
		if frame.leafL < diffIdx {
			stack = append(stack, routeFrame{
				node:  frame.node + 1,
				leafL: frame.leafL,
				leafR: diffIdx + 1,
			})
		}
	}
	return nil
}

func (n *node) insertSlotAbovePath(key []byte, diff uint16, wantLeaf int) (int, bool, error) {
	size := int(n.size)
	if size == 0 {
		return 0, false, nil
	}
	if wantLeaf < 0 || wantLeaf >= size {
		return 0, false, ErrCorruptLayout
	}

	routeIdx := 0
	leafBase := 0
	leafCount := size

	for leafCount > 1 {
		if routeIdx < 0 || routeIdx >= size-1 {
			return 0, false, ErrCorruptLayout
		}
		r := n.routes[routeIdx]
		leftCount := int(r.leftCount())
		if leftCount <= 0 || leftCount >= leafCount {
			return 0, false, ErrCorruptLayout
		}
		if diff < r.diff() {
			if getDiffBit(key, diff) == 0 {
				return leafBase, true, nil
			}
			return leafBase + leafCount, true, nil
		}

		if r.bit(key) == 0 {
			routeIdx++
			leafCount = leftCount
		} else {
			routeIdx += leftCount
			leafBase += leftCount
			leafCount -= leftCount
		}
	}

	if leafBase != wantLeaf {
		return 0, false, ErrCorruptLayout
	}
	return 0, false, nil
}

func (n *node) insertRoute(slot int, key []byte, diff uint16) error {
	size := int(n.size)
	if size <= 0 || size >= MaxNodeReps {
		return ErrCorruptLayout
	}

	var leftAncestors [MaxNodeReps]uint16
	leftAncestorCount := 0
	routeIdx := 0
	leafBase := 0
	leafCount := size

	for leafCount > 1 {
		if routeIdx < 0 || routeIdx >= size-1 {
			return ErrCorruptLayout
		}
		r := n.routes[routeIdx]
		leftCount := int(r.leftCount())
		if leftCount <= 0 || leftCount >= leafCount {
			return ErrCorruptLayout
		}

		if diff < r.diff() {
			newLeftCount := 1
			expectedSlot := leafBase
			if getDiffBit(key, diff) != 0 {
				newLeftCount = leafCount
				expectedSlot = leafBase + leafCount
			}
			if slot != expectedSlot {
				return ErrCorruptLayout
			}
			n.insertRouteAt(routeIdx, diff, uint16(newLeftCount), leftAncestors[:leftAncestorCount])
			return nil
		}

		if r.bit(key) == 0 {
			leftAncestors[leftAncestorCount] = uint16(routeIdx)
			leftAncestorCount++
			routeIdx++
			leafCount = leftCount
		} else {
			routeIdx += leftCount
			leafBase += leftCount
			leafCount -= leftCount
		}
	}

	expectedSlot := leafBase
	if getDiffBit(key, diff) != 0 {
		expectedSlot = leafBase + 1
	}
	if slot != expectedSlot {
		return ErrCorruptLayout
	}
	n.insertRouteAt(routeIdx, diff, 1, leftAncestors[:leftAncestorCount])
	return nil
}

func (n *node) insertRouteAt(routeIdx int, diff uint16, leftCount uint16, leftAncestors []uint16) {
	size := int(n.size)
	copy(n.routes[routeIdx+1:size], n.routes[routeIdx:size-1])
	n.routes[routeIdx] = makeRoute(diff, leftCount)
	for _, ancestor := range leftAncestors {
		n.routes[ancestor].incLeftCount()
	}
}

func (n *node) deleteRoute(slot int) error {
	size := int(n.size)
	if size <= 1 {
		return ErrCorruptLayout
	}
	if slot < 0 || slot >= size {
		return ErrCorruptLayout
	}

	var leftAncestors [MaxNodeReps]uint16
	leftAncestorCount := 0
	routeIdx := 0
	leafBase := 0
	leafCount := size
	parentRouteIdx := -1

	for leafCount > 1 {
		if routeIdx < 0 || routeIdx >= size-1 {
			return ErrCorruptLayout
		}
		r := n.routes[routeIdx]
		leftCount := int(r.leftCount())
		if leftCount <= 0 || leftCount >= leafCount {
			return ErrCorruptLayout
		}

		parentRouteIdx = routeIdx
		if slot < leafBase+leftCount {
			if leftCount > 1 {
				leftAncestors[leftAncestorCount] = uint16(routeIdx)
				leftAncestorCount++
			}
			routeIdx++
			leafCount = leftCount
		} else {
			routeIdx += leftCount
			leafBase += leftCount
			leafCount -= leftCount
		}
	}

	if leafBase != slot || parentRouteIdx < 0 {
		return ErrCorruptLayout
	}
	n.deleteRouteAt(parentRouteIdx, leftAncestors[:leftAncestorCount])
	return nil
}

func (n *node) deleteRouteAt(routeIdx int, leftAncestors []uint16) {
	size := int(n.size)
	copy(n.routes[routeIdx:], n.routes[routeIdx+1:size-1])
	n.routes[size-2] = route{}
	for _, ancestor := range leftAncestors {
		n.routes[ancestor].decLeftCount()
	}
}
