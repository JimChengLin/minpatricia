package minpatricia

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

	edgeSlot, err := idx.pathEdgeInsertSlot(frames, target, slot, size)
	if err != nil {
		return err
	}
	if err := idx.splitAndWriteNodeAt(targetID, reps, edgeSlot); err != nil {
		return err
	}
	return idx.propagateBoundary(frames, target, oldFirst, oldLast)
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

func (idx *Index) pathEdgeInsertSlot(frames []putFrame, target int, slot int, oldSize int) (int, error) {
	if slot != 0 && slot != oldSize {
		return -1, nil
	}
	for i := 0; i < target; i++ {
		if slot == 0 {
			if frames[i].leaf != 0 {
				return -1, nil
			}
			continue
		}
		n, err := idx.nodeByID(frames[i].id)
		if err != nil {
			return -1, err
		}
		if frames[i].leaf != int(n.size)-1 {
			return -1, nil
		}
	}
	return slot, nil
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
