package minpatricia

type ItemIterator func(key []byte, pos Position) bool

type iterBound struct {
	key []byte
	ok  bool
}

const iterStackDepth = 16

type iterPath struct {
	stack    [iterStackDepth]putFrame
	overflow []putFrame
	len      int
}

func (idx *Index) Ascend(fn ItemIterator) error {
	var path iterPath
	ok, err := idx.leftmostRoot(&path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.ascendFromPath(&path, iterBound{}, fn)
	return err
}

func (idx *Index) AscendRange(greaterOrEqual, lessThan []byte, fn ItemIterator) error {
	if err := checkKeySize(greaterOrEqual); err != nil {
		return err
	}
	if err := checkKeySize(lessThan); err != nil {
		return err
	}
	upper := iterBound{key: lessThan, ok: true}
	var path iterPath
	ok, err := idx.seekGreaterOrEqual(greaterOrEqual, &path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.ascendFromPath(&path, upper, fn)
	return err
}

func (idx *Index) AscendLessThan(pivot []byte, fn ItemIterator) error {
	if err := checkKeySize(pivot); err != nil {
		return err
	}
	upper := iterBound{key: pivot, ok: true}
	var path iterPath
	ok, err := idx.leftmostRoot(&path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.ascendFromPath(&path, upper, fn)
	return err
}

func (idx *Index) AscendGreaterOrEqual(pivot []byte, fn ItemIterator) error {
	if err := checkKeySize(pivot); err != nil {
		return err
	}
	var path iterPath
	ok, err := idx.seekGreaterOrEqual(pivot, &path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.ascendFromPath(&path, iterBound{}, fn)
	return err
}

func (idx *Index) Descend(fn ItemIterator) error {
	var path iterPath
	ok, err := idx.rightmostRoot(&path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.descendFromPath(&path, iterBound{}, fn)
	return err
}

func (idx *Index) DescendRange(lessOrEqual, greaterThan []byte, fn ItemIterator) error {
	if err := checkKeySize(lessOrEqual); err != nil {
		return err
	}
	if err := checkKeySize(greaterThan); err != nil {
		return err
	}
	lower := iterBound{key: greaterThan, ok: true}
	var path iterPath
	ok, err := idx.seekLessOrEqual(lessOrEqual, &path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.descendFromPath(&path, lower, fn)
	return err
}

func (idx *Index) DescendLessOrEqual(pivot []byte, fn ItemIterator) error {
	if err := checkKeySize(pivot); err != nil {
		return err
	}
	var path iterPath
	ok, err := idx.seekLessOrEqual(pivot, &path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.descendFromPath(&path, iterBound{}, fn)
	return err
}

func (idx *Index) DescendGreaterThan(pivot []byte, fn ItemIterator) error {
	if err := checkKeySize(pivot); err != nil {
		return err
	}
	lower := iterBound{key: pivot, ok: true}
	var path iterPath
	ok, err := idx.rightmostRoot(&path)
	if err != nil || !ok {
		return err
	}
	_, err = idx.descendFromPath(&path, lower, fn)
	return err
}

func (idx *Index) seekGreaterOrEqual(key []byte, path *iterPath) (bool, error) {
	path.reset()
	id := idx.rootID

	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return false, err
		}
		if n.size == 0 {
			return false, nil
		}

		leaf, ok := n.lookup(key)
		if !ok {
			return false, ErrCorruptLayout
		}
		path.pushNode(id, n, leaf)

		r := n.reps[leaf]
		if r.isChild() {
			id = r.childID()
			continue
		}

		pos := r.position()
		recordKey, err := idx.key(pos)
		if err != nil {
			return false, err
		}
		cmp, diff, err := compareAndDiffBit(key, recordKey)
		if err == ErrEqualKeys {
			return true, nil
		}
		if err != nil {
			return false, err
		}

		target, slot, err := idx.insertSlotFromPath(path.framesForInsert(), key, cmp, diff)
		if err != nil {
			return false, err
		}
		return idx.positionAtOrAfter(path, target, slot)
	}
}

func (idx *Index) seekLessOrEqual(key []byte, path *iterPath) (bool, error) {
	path.reset()
	id := idx.rootID

	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return false, err
		}
		if n.size == 0 {
			return false, nil
		}

		leaf, ok := n.lookup(key)
		if !ok {
			return false, ErrCorruptLayout
		}
		path.pushNode(id, n, leaf)

		r := n.reps[leaf]
		if r.isChild() {
			id = r.childID()
			continue
		}

		pos := r.position()
		recordKey, err := idx.key(pos)
		if err != nil {
			return false, err
		}
		cmp, diff, err := compareAndDiffBit(key, recordKey)
		if err == ErrEqualKeys {
			return true, nil
		}
		if err != nil {
			return false, err
		}

		target, slot, err := idx.insertSlotFromPath(path.framesForInsert(), key, cmp, diff)
		if err != nil {
			return false, err
		}
		return idx.positionAtOrBefore(path, target, slot)
	}
}

func (p *iterPath) reset() {
	p.len = 0
	p.overflow = p.overflow[:0]
}

func (p *iterPath) push(id uint64, leaf int) {
	p.pushNode(id, nil, leaf)
}

func (p *iterPath) pushNode(id uint64, n *node, leaf int) {
	frame := putFrame{id: id, node: n, leaf: leaf}
	if p.len < len(p.stack) {
		p.stack[p.len] = frame
	} else {
		p.overflow = append(p.overflow, frame)
	}
	p.len++
}

func (p *iterPath) at(i int) *putFrame {
	if i < len(p.stack) {
		return &p.stack[i]
	}
	return &p.overflow[i-len(p.stack)]
}

func (p *iterPath) truncate(n int) {
	if n <= len(p.stack) {
		p.overflow = p.overflow[:0]
	} else {
		p.overflow = p.overflow[:n-len(p.stack)]
	}
	p.len = n
}

func (p *iterPath) framesForInsert() []putFrame {
	if p.len <= len(p.stack) {
		return p.stack[:p.len]
	}

	frames := make([]putFrame, p.len)
	copy(frames, p.stack[:])
	copy(frames[len(p.stack):], p.overflow)
	return frames
}

func (idx *Index) nodeForFrame(frame *putFrame) (*node, error) {
	if frame.node != nil {
		return frame.node, nil
	}
	return idx.nodeForFrameSlow(frame)
}

// Keep the nil-node fallback out of nodeForFrame's inline budget so cached
// iterator frames do not pay a call on the FullSet Visit hot path.
//
//go:noinline
func (idx *Index) nodeForFrameSlow(frame *putFrame) (*node, error) {
	return idx.nodeByID(frame.id)
}

func (idx *Index) positionAtOrAfter(path *iterPath, target int, slot int) (bool, error) {
	if target < 0 || target >= path.len {
		return false, ErrCorruptLayout
	}
	path.truncate(target + 1)

	for {
		frame := path.at(path.len - 1)
		n, err := idx.nodeForFrame(frame)
		if err != nil {
			return false, err
		}
		size := int(n.size)
		if slot < 0 || slot > size {
			return false, ErrCorruptLayout
		}
		if slot < size {
			frame.leaf = slot
			r := n.reps[slot]
			if r.isChild() {
				return idx.leftmostRecord(path, r.childID())
			}
			return true, nil
		}

		if path.len == 1 {
			return false, nil
		}
		path.truncate(path.len - 1)
		slot = path.at(path.len-1).leaf + 1
	}
}

func (idx *Index) positionAtOrBefore(path *iterPath, target int, slot int) (bool, error) {
	if target < 0 || target >= path.len {
		return false, ErrCorruptLayout
	}
	path.truncate(target + 1)

	for {
		frame := path.at(path.len - 1)
		n, err := idx.nodeForFrame(frame)
		if err != nil {
			return false, err
		}
		size := int(n.size)
		if slot < 0 || slot > size {
			return false, ErrCorruptLayout
		}
		if slot > 0 {
			prev := slot - 1
			frame.leaf = prev
			r := n.reps[prev]
			if r.isChild() {
				return idx.rightmostRecord(path, r.childID())
			}
			return true, nil
		}

		if path.len == 1 {
			return false, nil
		}
		path.truncate(path.len - 1)
		slot = path.at(path.len - 1).leaf
	}
}

func (idx *Index) leftmostRoot(path *iterPath) (bool, error) {
	path.reset()
	root, err := idx.root()
	if err != nil {
		return false, err
	}
	if root.size == 0 {
		return false, nil
	}
	return idx.leftmostRecord(path, idx.rootID)
}

func (idx *Index) rightmostRoot(path *iterPath) (bool, error) {
	path.reset()
	root, err := idx.root()
	if err != nil {
		return false, err
	}
	if root.size == 0 {
		return false, nil
	}
	return idx.rightmostRecord(path, idx.rootID)
}

func (idx *Index) leftmostRecord(path *iterPath, id uint64) (bool, error) {
	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return false, err
		}
		if n.size == 0 {
			return false, ErrCorruptLayout
		}
		path.pushNode(id, n, 0)

		r := n.reps[0]
		if r.isChild() {
			id = r.childID()
			continue
		}
		return true, nil
	}
}

func (idx *Index) rightmostRecord(path *iterPath, id uint64) (bool, error) {
	for {
		n, err := idx.nodeByID(id)
		if err != nil {
			return false, err
		}
		if n.size == 0 {
			return false, ErrCorruptLayout
		}
		leaf := int(n.size) - 1
		path.pushNode(id, n, leaf)

		r := n.reps[leaf]
		if r.isChild() {
			id = r.childID()
			continue
		}
		return true, nil
	}
}

func (idx *Index) nextPath(path *iterPath) (bool, error) {
	for path.len > 0 {
		frame := path.at(path.len - 1)
		n, err := idx.nodeForFrame(frame)
		if err != nil {
			return false, err
		}
		next := frame.leaf + 1
		if next < int(n.size) {
			frame.leaf = next
			r := n.reps[next]
			if r.isChild() {
				return idx.leftmostRecord(path, r.childID())
			}
			return true, nil
		}
		path.truncate(path.len - 1)
	}
	return false, nil
}

func (idx *Index) prevPath(path *iterPath) (bool, error) {
	for path.len > 0 {
		frame := path.at(path.len - 1)
		n, err := idx.nodeForFrame(frame)
		if err != nil {
			return false, err
		}
		prev := frame.leaf - 1
		if prev >= 0 {
			frame.leaf = prev
			r := n.reps[prev]
			if r.isChild() {
				return idx.rightmostRecord(path, r.childID())
			}
			return true, nil
		}
		path.truncate(path.len - 1)
	}
	return false, nil
}

func (idx *Index) currentRecord(path *iterPath) ([]byte, Position, error) {
	if path.len == 0 {
		return nil, 0, ErrCorruptLayout
	}
	frame := path.at(path.len - 1)
	n, err := idx.nodeForFrame(frame)
	if err != nil {
		return nil, 0, err
	}
	if frame.leaf < 0 || frame.leaf >= int(n.size) {
		return nil, 0, ErrCorruptLayout
	}
	r := n.reps[frame.leaf]
	if r.isChild() {
		return nil, 0, ErrCorruptLayout
	}
	pos := r.position()
	key, err := idx.key(pos)
	if err != nil {
		return nil, 0, err
	}
	return key, pos, nil
}

func (idx *Index) ascendFromPath(path *iterPath, upper iterBound, fn ItemIterator) (bool, error) {
	for path.len > 0 {
		key, pos, err := idx.currentRecord(path)
		if err != nil {
			return false, err
		}
		if upper.ok && compareKeys(key, upper.key) >= 0 {
			return false, nil
		}
		if !fn(key, pos) {
			return false, nil
		}
		ok, err := idx.nextPath(path)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func (idx *Index) descendFromPath(path *iterPath, lower iterBound, fn ItemIterator) (bool, error) {
	for path.len > 0 {
		key, pos, err := idx.currentRecord(path)
		if err != nil {
			return false, err
		}
		if lower.ok && compareKeys(key, lower.key) <= 0 {
			return false, nil
		}
		if !fn(key, pos) {
			return false, nil
		}
		ok, err := idx.prevPath(path)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}
