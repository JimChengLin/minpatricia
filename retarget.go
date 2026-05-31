package minpatricia

func (idx *Index) retarget(key []byte, oldPos Position, newRep rep) error {
	var frameBuf [16]putFrame
	frames := frameBuf[:0]
	id := idx.rootID

	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return err
		}
		leaf, ok := n.lookup(key)
		if !ok {
			return ErrPositionMismatch
		}
		frames = append(frames, putFrame{id: id, leaf: leaf})

		r := n.reps[leaf]
		if r.isChild() {
			id = r.childID()
			continue
		}
		if r.position() != oldPos {
			return ErrPositionMismatch
		}

		oldFirst, oldLast := n.firstPos, n.lastPos
		newPos := newRep.position()
		// Retarget only accepts record reps, so the boundary position is newPos.
		if leaf == 0 {
			n.firstPos = newPos
		}
		if leaf == int(n.size)-1 {
			n.lastPos = newPos
		}
		n.reps[leaf] = newRep
		return idx.propagateBoundary(frames, len(frames)-1, oldFirst, oldLast)
	}
}
