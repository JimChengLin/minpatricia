package minpatricia

const (
	NodeSize    = 4096
	MaxNodeReps = 339
)

const childTag uint64 = 1 << 63

type Position uint64

type rep uint64

func makeRecordRep(pos Position) (rep, error) {
	if uint64(pos)&childTag != 0 {
		return 0, ErrPositionTag
	}
	return rep(pos), nil
}

func (r rep) isChild() bool {
	return uint64(r)&childTag != 0
}

func (r rep) position() Position {
	return Position(uint64(r) &^ childTag)
}

func makeChildRep(id uint64) (rep, error) {
	if id&childTag != 0 {
		return 0, ErrPositionTag
	}
	return rep(childTag | id), nil
}

func (r rep) childID() uint64 {
	return uint64(r) &^ childTag
}

type route struct {
	diff      uint16
	leftCount uint16
}

// NodePage is the fixed-size opaque page stored by NodeStore.
type NodePage = node

type node struct {
	size     uint16
	firstPos Position
	lastPos  Position
	routes   [MaxNodeReps - 1]route
	reps     [MaxNodeReps]rep
	_        [NodeSize - 4088]byte
}

func layoutBytes(repCount int) int {
	if repCount <= 0 {
		return 24
	}
	return alignUp(24+(repCount-1)*4, 8) + repCount*8
}

func alignUp(v, align int) int {
	return (v + align - 1) &^ (align - 1)
}
