package minpatricia

func (idx *Index) mergeChildIntoParent(n *node, slot int, childID uint64, child *node) error {
	parentSize := int(n.size)
	childSize := int(child.size)
	newSize := parentSize - 1 + childSize

	var oldParentDiffBuf [MaxNodeReps - 1]uint16
	oldParentDiffs := oldParentDiffBuf[:parentSize-1]
	if err := n.routeDiffs(oldParentDiffs); err != nil {
		return err
	}

	var newDiffBuf [MaxNodeReps - 1]uint16
	newDiffs := newDiffBuf[:newSize-1]
	copy(newDiffs[:slot], oldParentDiffs[:slot])
	if childSize > 1 {
		if err := child.routeDiffs(newDiffs[slot : slot+childSize-1]); err != nil {
			return err
		}
	}
	copy(newDiffs[slot+childSize-1:], oldParentDiffs[slot:])

	copy(n.reps[slot+childSize:], n.reps[slot+1:parentSize])
	copy(n.reps[slot:slot+childSize], child.reps[:childSize])
	n.size = uint16(newSize)
	if err := idx.rebuildNodeWithDiffs(n, newDiffs); err != nil {
		return err
	}
	return idx.freeNode(childID)
}

func (idx *Index) mergeChildWithSibling(n *node, slot int, childID uint64, child *node) (bool, error) {
	parentSize := int(n.size)
	childSize := int(child.size)
	leftSlot := slot - 1
	rightSlot := slot + 1
	leftSize := -1
	rightSize := -1
	var leftID, rightID uint64
	var left, right *node

	if leftSlot >= 0 {
		r := n.reps[leftSlot]
		if r.isChild() {
			var err error
			leftID = r.childID()
			left, err = idx.nodeByID(leftID)
			if err != nil {
				return false, err
			}
			leftSize = int(left.size)
		}
	}
	if rightSlot < parentSize {
		r := n.reps[rightSlot]
		if r.isChild() {
			var err error
			rightID = r.childID()
			right, err = idx.nodeByID(rightID)
			if err != nil {
				return false, err
			}
			rightSize = int(right.size)
		}
	}

	canMergeLeft := leftSize >= 0 && leftSize+childSize <= MaxNodeReps && n.isDirectRoutePair(leftSlot)
	canMergeRight := rightSize >= 0 && childSize+rightSize <= MaxNodeReps && n.isDirectRoutePair(slot)
	if !canMergeLeft && !canMergeRight {
		return false, nil
	}
	if canMergeRight && (!canMergeLeft || rightSize > leftSize) {
		return true, idx.mergeRightSibling(n, slot, childID, child, rightSlot, rightID, right)
	}
	return true, idx.mergeLeftSibling(n, leftSlot, leftID, left, slot, childID, child)
}

func (n *node) isDirectRoutePair(diffIdx int) bool {
	size := int(n.size)
	if diffIdx < 0 || diffIdx >= size-1 {
		return false
	}

	var stackBuf [MaxNodeReps]routeFrame
	stack := stackBuf[:1]
	stack[0] = routeFrame{node: 0, leafL: 0, leafR: size}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		r := n.routes[frame.node]
		leftCount := int(r.leftCount())
		currentDiff := frame.leafL + leftCount - 1
		if currentDiff == diffIdx {
			return frame.leafL == diffIdx && frame.leafR == diffIdx+2
		}

		rightIdx := frame.node + leftCount
		if currentDiff+1 < frame.leafR-1 {
			stack = append(stack, routeFrame{
				node:  rightIdx,
				leafL: currentDiff + 1,
				leafR: frame.leafR,
			})
		}
		if frame.leafL < currentDiff {
			stack = append(stack, routeFrame{
				node:  frame.node + 1,
				leafL: frame.leafL,
				leafR: currentDiff + 1,
			})
		}
	}
	return false
}

func (idx *Index) mergeLeftSibling(n *node, leftSlot int, leftID uint64, left *node, slot int, childID uint64, child *node) error {
	parentSize := int(n.size)
	leftSize := int(left.size)
	childSize := int(child.size)
	var buf [MaxNodeReps]rep
	reps := buf[:leftSize+childSize]
	copy(reps, left.reps[:leftSize])
	copy(reps[leftSize:], child.reps[:childSize])

	var oldParentDiffBuf [MaxNodeReps - 1]uint16
	oldParentDiffs := oldParentDiffBuf[:parentSize-1]
	if err := n.routeDiffs(oldParentDiffs); err != nil {
		return err
	}

	var diffBuf [MaxNodeReps - 1]uint16
	diffs := diffBuf[:len(reps)-1]
	if leftSize > 1 {
		if err := left.routeDiffs(diffs[:leftSize-1]); err != nil {
			return err
		}
	}
	diffs[leftSize-1] = oldParentDiffs[leftSlot]
	if childSize > 1 {
		if err := child.routeDiffs(diffs[leftSize:]); err != nil {
			return err
		}
	}

	if err := idx.writeNodeWithDiffs(leftID, reps, diffs); err != nil {
		return err
	}
	if err := idx.removeMergedParentRep(n, slot, leftSlot); err != nil {
		return err
	}
	return idx.freeNode(childID)
}

func (idx *Index) mergeRightSibling(n *node, slot int, childID uint64, child *node, rightSlot int, rightID uint64, right *node) error {
	parentSize := int(n.size)
	childSize := int(child.size)
	rightSize := int(right.size)
	var buf [MaxNodeReps]rep
	reps := buf[:childSize+rightSize]
	copy(reps, child.reps[:childSize])
	copy(reps[childSize:], right.reps[:rightSize])

	var oldParentDiffBuf [MaxNodeReps - 1]uint16
	oldParentDiffs := oldParentDiffBuf[:parentSize-1]
	if err := n.routeDiffs(oldParentDiffs); err != nil {
		return err
	}

	var diffBuf [MaxNodeReps - 1]uint16
	diffs := diffBuf[:len(reps)-1]
	if childSize > 1 {
		if err := child.routeDiffs(diffs[:childSize-1]); err != nil {
			return err
		}
	}
	diffs[childSize-1] = oldParentDiffs[slot]
	if rightSize > 1 {
		if err := right.routeDiffs(diffs[childSize:]); err != nil {
			return err
		}
	}

	if err := idx.writeNodeWithDiffs(childID, reps, diffs); err != nil {
		return err
	}
	if err := idx.removeMergedParentRep(n, rightSlot, slot); err != nil {
		return err
	}
	return idx.freeNode(rightID)
}

func (idx *Index) removeMergedParentRep(n *node, slot int, dropDiff int) error {
	size := int(n.size)
	if slot < 0 || slot >= size {
		return ErrCorruptLayout
	}
	if dropDiff < 0 || dropDiff >= size-1 {
		return ErrCorruptLayout
	}

	var oldDiffBuf [MaxNodeReps - 1]uint16
	oldDiffs := oldDiffBuf[:size-1]
	if err := n.routeDiffs(oldDiffs); err != nil {
		return err
	}
	var newDiffBuf [MaxNodeReps - 1]uint16
	newDiffs := newDiffBuf[:size-2]
	copy(newDiffs, oldDiffs[:dropDiff])
	copy(newDiffs[dropDiff:], oldDiffs[dropDiff+1:])

	copy(n.reps[slot:], n.reps[slot+1:size])
	n.reps[size-1] = 0
	n.size = uint16(size - 1)
	return idx.rebuildNodeWithDiffs(n, newDiffs)
}
