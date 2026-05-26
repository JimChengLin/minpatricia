package minpatricia

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
			if err := idx.deleteAtIncremental(n, leaf); err != nil {
				return 0, false, err
			}
			return oldPos, true, nil
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
		if err := idx.deleteAtIncremental(n, slot); err != nil {
			return 0, false, err
		}
		if err := idx.freeNode(childID); err != nil {
			return 0, false, err
		}
		return pos, true, nil
	}

	childSize := int(child.size)
	if parentSize-1+childSize <= MaxNodeReps {
		return pos, true, idx.mergeChildIntoParent(n, slot, childID, child)
	}

	if childSize < MaxNodeReps/32 {
		merged, err := idx.mergeChildWithSibling(n, slot, childID, child)
		if err != nil || merged {
			return pos, true, err
		}
	}

	if child.firstPos == oldFirst && child.lastPos == oldLast {
		return pos, true, nil
	}
	return pos, true, idx.rebuildParentAfterChildBoundary(n, slot, child, oldFirst, oldLast)
}

func (idx *Index) deleteAtIncremental(n *node, slot int) error {
	size := int(n.size)
	if slot < 0 || slot >= size {
		return ErrCorruptLayout
	}

	if size == 1 {
		n.reps[0] = 0
		n.size = 0
		n.firstPos = 0
		n.lastPos = 0
		return nil
	}

	firstPos, lastPos := n.firstPos, n.lastPos
	var err error
	if slot == 0 {
		firstPos, err = idx.minPos(n.reps[1])
		if err != nil {
			return err
		}
	}
	if slot == size-1 {
		lastPos, err = idx.maxPos(n.reps[size-2])
		if err != nil {
			return err
		}
	}
	if err := n.deleteRoute(slot); err != nil {
		return err
	}

	copy(n.reps[slot:], n.reps[slot+1:size])
	n.reps[size-1] = 0
	n.size = uint16(size - 1)
	n.firstPos = firstPos
	n.lastPos = lastPos
	return nil
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
