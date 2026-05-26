package minpatricia

const (
	// Smaller legal subtrees are left to the balanced fallback instead of
	// creating tiny child pages that barely relieve parent pressure.
	preferredSplitMinRatioNum = 1
	preferredSplitMinRatioDen = 3
)

func (idx *Index) promoteSibling(parentID uint64, childSlot int, childID uint64, reps []rep) error {
	if len(reps) != MaxNodeReps+1 {
		return ErrCorruptLayout
	}

	parent, err := idx.nodeByID(parentID)
	if err != nil {
		return err
	}
	parentSize := int(parent.size)
	if childSlot < 0 || childSlot >= parentSize || parentSize >= MaxNodeReps {
		return ErrCorruptLayout
	}
	childRep := parent.reps[childSlot]
	if !childRep.isChild() || childRep.childID() != childID {
		return ErrCorruptLayout
	}

	diffs, err := idx.buildDiffs(reps)
	if err != nil {
		return err
	}
	_, _, root := idx.buildCartesian(diffs)
	split := root + 1
	splitDiff := diffs[root]

	siblingID, _, err := idx.allocNode()
	if err != nil {
		return err
	}
	if err := idx.writeNodeWithDiffs(childID, reps[:split], diffs[:split-1]); err != nil {
		return err
	}
	if err := idx.writeNodeWithDiffs(siblingID, reps[split:], diffs[split:]); err != nil {
		return err
	}

	siblingRep, err := makeChildRep(siblingID)
	if err != nil {
		return err
	}

	var parentReps [MaxNodeReps]rep
	newParentReps := parentReps[:parentSize+1]
	copy(newParentReps, parent.reps[:childSlot+1])
	newParentReps[childSlot+1] = siblingRep
	copy(newParentReps[childSlot+2:], parent.reps[childSlot+1:parentSize])

	var oldDiffBuf [MaxNodeReps - 1]uint16
	oldDiffs := oldDiffBuf[:parentSize-1]
	if err := parent.routeDiffs(oldDiffs); err != nil {
		return err
	}

	var newDiffBuf [MaxNodeReps - 1]uint16
	newDiffs := newDiffBuf[:parentSize]
	copy(newDiffs, oldDiffs[:childSlot])
	newDiffs[childSlot] = splitDiff
	copy(newDiffs[childSlot+1:], oldDiffs[childSlot:])

	return idx.writeNodeWithDiffs(parentID, newParentReps, newDiffs)
}

func (idx *Index) writeNodeWithDiffs(id uint64, reps []rep, diffs []uint16) error {
	if len(reps) > MaxNodeReps || len(diffs) != len(reps)-1 {
		return ErrCorruptLayout
	}

	n, err := idx.nodeByID(id)
	if err != nil {
		return err
	}
	for i := range n.reps {
		n.reps[i] = 0
	}
	copy(n.reps[:], reps)
	n.size = uint16(len(reps))
	return idx.rebuildNodeWithDiffs(n, diffs)
}

func (idx *Index) writeNode(id uint64, reps []rep) error {
	if len(reps) > MaxNodeReps {
		return idx.splitAndWriteNode(id, reps)
	}

	n, err := idx.nodeByID(id)
	if err != nil {
		return err
	}
	for i := range n.reps {
		n.reps[i] = 0
	}
	copy(n.reps[:], reps)
	n.size = uint16(len(reps))
	return idx.rebuildNode(n)
}

func (idx *Index) splitAndWriteNode(id uint64, reps []rep) error {
	return idx.splitAndWriteNodeAt(id, reps, -1)
}

func (idx *Index) splitAndWriteNodeAt(id uint64, reps []rep, insertSlot int) error {
	for len(reps) > MaxNodeReps {
		start, count, err := idx.chooseSplitRange(reps, insertSlot)
		if err != nil {
			return err
		}
		end := start + count

		childID, child, err := idx.allocNode()
		if err != nil {
			return err
		}
		if err := idx.writeNode(childID, reps[start:end]); err != nil {
			return err
		}
		if child.size == 0 {
			return ErrCorruptLayout
		}

		childRep, err := makeChildRep(childID)
		if err != nil {
			return err
		}

		reps[start] = childRep
		copy(reps[start+1:], reps[end:])
		reps = reps[:len(reps)-count+1]
		insertSlot = remapInsertSlotAfterSplit(insertSlot, start, end, count)
	}
	return idx.writeNode(id, reps)
}

func remapInsertSlotAfterSplit(slot, start, end, count int) int {
	if slot < 0 {
		return slot
	}
	if slot < start {
		return slot
	}
	if slot < end {
		return start
	}
	return slot - count + 1
}

func (idx *Index) chooseSplitRange(reps []rep, insertSlot int) (int, int, error) {
	if len(reps) < 4 {
		return 0, 0, ErrCorruptLayout
	}

	diffs, err := idx.buildDiffs(reps)
	if err != nil {
		return 0, 0, err
	}
	if insertSlot == 0 || insertSlot == len(reps)-1 {
		start, count, ok := idx.chooseEdgeSplitRange(diffs, len(reps), insertSlot)
		if ok {
			return start, count, nil
		}
	}
	left, right, root := idx.buildCartesian(diffs)

	target := len(reps) / 2
	minPreferredCount := (len(reps)*preferredSplitMinRatioNum + preferredSplitMinRatioDen - 1) / preferredSplitMinRatioDen
	bestStart := -1
	bestCount := 0
	bestScore := len(reps)
	preferredStart := -1
	preferredCount := 0

	stack := []routeFrame{{node: root, leafL: 0, leafR: len(reps)}}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		count := frame.leafR - frame.leafL
		if count >= 2 && count <= len(reps)-2 {
			score := absInt(count - target)
			if bestStart == -1 || score < bestScore {
				bestStart = frame.leafL
				bestCount = count
				bestScore = score
			}
			if count >= minPreferredCount && count <= target && (preferredStart == -1 || count > preferredCount) {
				preferredStart = frame.leafL
				preferredCount = count
			}
		}

		i := frame.node
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

	if bestStart == -1 {
		count := len(reps) / 2
		start := (len(reps) - count) / 2
		return start, count, nil
	}
	// Prefer the largest legal subtree that is big enough but no larger than
	// half a page; otherwise fall back to the original balanced choice.
	if preferredStart != -1 {
		return preferredStart, preferredCount, nil
	}
	return bestStart, bestCount, nil
}

func (idx *Index) chooseEdgeSplitRange(diffs []uint16, repCount int, insertSlot int) (int, int, bool) {
	left, right, root := idx.buildCartesian(diffs)
	bestStart := -1
	bestCount := 0

	stack := []routeFrame{{node: root, leafL: 0, leafR: repCount}}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		count := frame.leafR - frame.leafL
		if count >= 2 && count <= repCount-2 && (insertSlot < frame.leafL || insertSlot >= frame.leafR) {
			if bestStart == -1 || count > bestCount {
				bestStart = frame.leafL
				bestCount = count
			}
		}

		i := frame.node
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

	return bestStart, bestCount, bestStart != -1
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
