package minpatricia

type putFrame struct {
	id   uint64
	leaf int
}

func (idx *Index) insertOrReplace(key []byte, newRep rep) (Position, bool, error) {
	var frameBuf [16]putFrame
	frames := frameBuf[:0]
	id := idx.rootID

	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return 0, false, err
		}
		if n.size == 0 {
			if err := idx.insertAtIncremental(id, 0, newRep, key, 0); err != nil {
				return 0, false, err
			}
			return 0, false, nil
		}

		leaf, ok := n.lookup(key)
		if !ok {
			return 0, false, ErrCorruptLayout
		}
		frames = append(frames, putFrame{id: id, leaf: leaf})

		r := n.reps[leaf]
		if r.isChild() {
			id = r.childID()
			continue
		}

		oldPos := r.position()
		recordKey, err := idx.key(oldPos)
		if err != nil {
			return 0, false, err
		}
		cmp := compareKeys(key, recordKey)
		if cmp == 0 {
			n.reps[leaf] = newRep
			return oldPos, true, nil
		}

		diff, err := findDiffBit(key, recordKey)
		if err != nil {
			return 0, false, err
		}
		target, slot, err := idx.insertSlotFromPath(frames, key, cmp, diff)
		if err != nil {
			return 0, false, err
		}
		if err := idx.insertAtPath(frames, target, slot, newRep, key, diff); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
}

func (idx *Index) insertSlotFromPath(frames []putFrame, key []byte, cmp int, diff uint16) (int, int, error) {
	for i, frame := range frames {
		n, err := idx.nodeByID(frame.id)
		if err != nil {
			return 0, 0, err
		}
		slot, ok, err := n.insertSlotAbovePath(key, diff, frame.leaf)
		if err != nil || ok {
			return i, slot, err
		}
	}

	last := frames[len(frames)-1]
	if cmp < 0 {
		return len(frames) - 1, last.leaf, nil
	}
	return len(frames) - 1, last.leaf + 1, nil
}

func (idx *Index) insertAtPath(frames []putFrame, target int, slot int, newRep rep, key []byte, diff uint16) error {
	targetID := frames[target].id
	n, err := idx.nodeByID(targetID)
	if err != nil {
		return err
	}
	oldFirst, oldLast := n.firstPos, n.lastPos

	size := int(n.size)
	if size < MaxNodeReps {
		if err := idx.insertAtIncremental(targetID, slot, newRep, key, diff); err != nil {
			return err
		}
		return idx.propagateBoundary(frames, target, oldFirst, oldLast)
	}

	var buf [MaxNodeReps + 1]rep
	reps := buf[:size+1]
	copy(reps, n.reps[:slot])
	reps[slot] = newRep
	copy(reps[slot+1:], n.reps[slot:size])

	if target > 0 {
		parentFrame := frames[target-1]
		parent, err := idx.nodeByID(parentFrame.id)
		if err != nil {
			return err
		}
		if int(parent.size) < MaxNodeReps {
			parentOldFirst, parentOldLast := parent.firstPos, parent.lastPos
			if err := idx.promoteSibling(parentFrame.id, parentFrame.leaf, targetID, reps); err != nil {
				return err
			}
			return idx.propagateBoundary(frames, target-1, parentOldFirst, parentOldLast)
		}
	}

	if err := idx.splitAndWriteNode(targetID, reps); err != nil {
		return err
	}
	return idx.propagateBoundary(frames, target, oldFirst, oldLast)
}

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

func (idx *Index) insertAtIncremental(id uint64, slot int, newRep rep, key []byte, diff uint16) error {
	n, err := idx.nodeByID(id)
	if err != nil {
		return err
	}
	size := int(n.size)
	if slot < 0 || slot > size {
		return ErrCorruptLayout
	}
	if size >= MaxNodeReps {
		return idx.insertFullAt(id, n, slot, newRep)
	}

	firstPos, lastPos := n.firstPos, n.lastPos
	if size == 0 {
		firstPos, err = idx.minPos(newRep)
		if err != nil {
			return err
		}
		lastPos, err = idx.maxPos(newRep)
		if err != nil {
			return err
		}
	} else {
		if slot == 0 {
			firstPos, err = idx.minPos(newRep)
			if err != nil {
				return err
			}
		}
		if slot == size {
			lastPos, err = idx.maxPos(newRep)
			if err != nil {
				return err
			}
		}
		if err := n.insertRoute(slot, key, diff); err != nil {
			return err
		}
	}

	copy(n.reps[slot+1:], n.reps[slot:size])
	n.reps[slot] = newRep
	n.size = uint16(size + 1)
	n.firstPos = firstPos
	n.lastPos = lastPos
	return nil
}

func (idx *Index) insertFullAt(id uint64, n *node, slot int, newRep rep) error {
	size := int(n.size)
	var buf [MaxNodeReps + 1]rep
	reps := buf[:size+1]
	copy(reps, n.reps[:slot])
	reps[slot] = newRep
	copy(reps[slot+1:], n.reps[slot:size])
	return idx.splitAndWriteNode(id, reps)
}

func (idx *Index) propagateBoundary(frames []putFrame, changed int, oldFirst, oldLast Position) error {
	child, err := idx.nodeByID(frames[changed].id)
	if err != nil {
		return err
	}
	firstChanged := child.firstPos != oldFirst
	lastChanged := child.lastPos != oldLast
	if !firstChanged && !lastChanged {
		return nil
	}

	for i := changed - 1; i >= 0; i-- {
		parent, err := idx.nodeByID(frames[i].id)
		if err != nil {
			return err
		}
		childSlot := frames[i].leaf
		size := int(parent.size)
		if childSlot < 0 || childSlot >= size {
			return ErrCorruptLayout
		}
		if !parent.reps[childSlot].isChild() || parent.reps[childSlot].childID() != frames[i+1].id {
			return ErrCorruptLayout
		}

		parentOldFirst, parentOldLast := parent.firstPos, parent.lastPos
		if firstChanged && childSlot == 0 {
			parent.firstPos = child.firstPos
		}
		if lastChanged && childSlot == size-1 {
			parent.lastPos = child.lastPos
		}

		child = parent
		firstChanged = parent.firstPos != parentOldFirst
		lastChanged = parent.lastPos != parentOldLast
		if !firstChanged && !lastChanged {
			return nil
		}
	}
	return nil
}

func (idx *Index) deleteFrom(id uint64, key []byte) (Position, bool, error) {
	n, err := idx.nodeByID(id)
	if err != nil {
		return 0, false, err
	}

	leaf, ok := n.lookup(key)
	if !ok {
		return 0, false, nil
	}
	r := n.reps[leaf]

	if !r.isChild() {
		oldPos := r.position()
		recordKey, err := idx.key(oldPos)
		if err != nil {
			return 0, false, err
		}
		if compareKeys(recordKey, key) == 0 {
			size := int(n.size)
			var diffBuf [MaxNodeReps - 1]uint16
			newDiffs := diffBuf[:0]
			if size > 1 {
				newDiffs = diffBuf[:size-2]
			}
			if err := idx.deleteDiffs(n, leaf, newDiffs); err != nil {
				return 0, false, err
			}
			copy(n.reps[leaf:], n.reps[leaf+1:size])
			n.reps[size-1] = 0
			n.size = uint16(size - 1)
			return oldPos, true, idx.rebuildNodeWithDiffs(n, newDiffs)
		}
	} else {
		childID := r.childID()
		return idx.deleteFromChild(n, leaf, childID, key)
	}

	return 0, false, nil
}

func (idx *Index) deleteFromChild(n *node, slot int, childID uint64, key []byte) (Position, bool, error) {
	child, err := idx.nodeByID(childID)
	if err != nil {
		return 0, false, err
	}
	if child.size == 0 {
		return 0, false, ErrCorruptLayout
	}
	oldFirst, oldLast := child.firstPos, child.lastPos

	pos, deleted, err := idx.deleteFrom(childID, key)
	if err != nil || !deleted {
		return pos, deleted, err
	}
	parentSize := int(n.size)
	if child.size == 0 {
		var diffBuf [MaxNodeReps - 1]uint16
		newDiffs := diffBuf[:0]
		if parentSize > 1 {
			newDiffs = diffBuf[:parentSize-2]
		}
		if err := idx.deleteDiffs(n, slot, newDiffs); err != nil {
			return 0, false, err
		}
		copy(n.reps[slot:], n.reps[slot+1:parentSize])
		n.reps[parentSize-1] = 0
		n.size = uint16(parentSize - 1)
		if err := idx.freeNode(childID); err != nil {
			return 0, false, err
		}
		return pos, true, idx.rebuildNodeWithDiffs(n, newDiffs)
	}

	childSize := int(child.size)
	if parentSize-1+childSize <= MaxNodeReps {
		copy(n.reps[slot+childSize:], n.reps[slot+1:parentSize])
		copy(n.reps[slot:slot+childSize], child.reps[:childSize])
		n.size = uint16(parentSize - 1 + childSize)
		if err := idx.freeNode(childID); err != nil {
			return 0, false, err
		}
		return pos, true, idx.rebuildNode(n)
	}

	if child.firstPos == oldFirst && child.lastPos == oldLast {
		return pos, true, nil
	}
	return pos, true, idx.rebuildParentAfterChildBoundary(n, slot, child, oldFirst, oldLast)
}

func (idx *Index) rebuildParentAfterChildBoundary(n *node, slot int, child *node, oldFirst, oldLast Position) error {
	size := int(n.size)
	firstChanged := child.firstPos != oldFirst
	lastChanged := child.lastPos != oldLast
	if firstChanged && slot == 0 {
		n.firstPos = child.firstPos
	}
	if lastChanged && slot == size-1 {
		n.lastPos = child.lastPos
	}
	return nil
}

func (idx *Index) deleteDiffs(n *node, slot int, newDiffs []uint16) error {
	size := int(n.size)
	if slot < 0 || slot >= size {
		return ErrCorruptLayout
	}
	if size <= 1 {
		if len(newDiffs) != 0 {
			return ErrCorruptLayout
		}
		return nil
	}
	if len(newDiffs) != size-2 {
		return ErrCorruptLayout
	}

	var oldDiffBuf [MaxNodeReps - 1]uint16
	oldDiffs := oldDiffBuf[:size-1]
	if err := n.routeDiffs(oldDiffs); err != nil {
		return err
	}

	if slot > 1 {
		copy(newDiffs[:slot-1], oldDiffs[:slot-1])
	}
	if slot > 0 && slot < size-1 {
		diff, err := idx.diffBetweenReps(n.reps[slot-1], n.reps[slot+1])
		if err != nil {
			return err
		}
		newDiffs[slot-1] = diff
	}
	if slot < size-2 {
		copy(newDiffs[slot:], oldDiffs[slot+1:])
	}
	return nil
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
	for len(reps) > MaxNodeReps {
		start, count, err := idx.chooseSplitRange(reps)
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
	}
	return idx.writeNode(id, reps)
}

func (idx *Index) chooseSplitRange(reps []rep) (int, int, error) {
	if len(reps) < 4 {
		return 0, 0, ErrCorruptLayout
	}

	diffs, err := idx.buildDiffs(reps)
	if err != nil {
		return 0, 0, err
	}
	left, right, root := idx.buildCartesian(diffs)

	target := len(reps) / 2
	bestStart := -1
	bestCount := 0
	bestScore := len(reps)

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
	return bestStart, bestCount, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (idx *Index) compressRoot() error {
	root, err := idx.root()
	if err != nil {
		return err
	}
	for root.size == 1 && root.reps[0].isChild() {
		childID := root.reps[0].childID()
		child, err := idx.nodeByID(childID)
		if err != nil {
			return err
		}
		*root = *child
		if err := idx.freeNode(childID); err != nil {
			return err
		}
	}
	return nil
}

func (idx *Index) countRecords(n *node) (int, error) {
	total := 0
	size := int(n.size)
	for i := 0; i < size; i++ {
		r := n.reps[i]
		if !r.isChild() {
			total++
			continue
		}
		child, err := idx.nodeByID(r.childID())
		if err != nil {
			return 0, err
		}
		childCount, err := idx.countRecords(child)
		if err != nil {
			return 0, err
		}
		total += childCount
	}
	return total, nil
}
