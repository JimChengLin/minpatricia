package minpatricia

type putFrame struct {
	id   uint64
	node *node
	leaf int
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
