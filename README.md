# minpatricia

`minpatricia` is a compact ordered Patricia index for Go.

It stores only opaque `Position` handles and child node IDs. Keys and payloads
stay in a caller-owned `RecordStore`; node pages live behind a `NodeStore`.
The default implementation uses Go heap storage, while the index layout is
kept friendly to a future mmap-backed node store.

## Install

```sh
go get github.com/JimChengLin/minpatricia
```

## Use

```go
package main

import "github.com/JimChengLin/minpatricia"

func main() {
	idx, records := minpatricia.NewHeap[string]()

	pos := records.Add([]byte("alpha"), "payload")
	if _, _, err := idx.Put([]byte("alpha"), pos); err != nil {
		panic(err)
	}

	found, ok, err := idx.Get([]byte("alpha"))
	if err != nil {
		panic(err)
	}
	if ok {
		value, _ := records.Value(found)
		_ = value
	}

	_ = idx.AscendRange([]byte("a"), []byte("z"), func(key []byte, pos minpatricia.Position) bool {
		_, _ = key, pos
		return true
	})
}
```

Use `NewWithRecords(records)` when records live in your own store.

## Benchmarks

Apple M1 Pro, Go 1.25.1,
`MIN_PATRICIA_BENCH_LARGE=1 go test -run ^$ -bench . -benchmem -benchtime=500ms -count=1`.
The table reports the 100K-key rows from the full benchmark suite.

| operation | go map | google/btree | minpatricia |
|---|---:|---:|---:|
| Get | 17.85 ns/op | 269.8 ns/op | 132.2 ns/op |
| Seek >= | - | 266.5 ns/op | 153.1 ns/op |
| Seek <= | - | 283.6 ns/op | 153.1 ns/op |
| Replace | 23.79 ns/op | 281.4 ns/op | 139.6 ns/op |
| Build insert, per key | 21.84 ns/op | 311.3 ns/op | 568.6 ns/op |
| Visit FullSet Ordered | 19,764,860 ns/op | 273,935 ns/op | 961,726 ns/op |
| Visit FullSet Reverse | 23,854,895 ns/op | 267,344 ns/op | 1,020,794 ns/op |
| Delete-heavy | 55.70 ns/op | 208.6 ns/op | 171.1 ns/op |

Node-store footprint for the same 100K-key benchmark. Node size is 4096
bytes and each node can hold up to 339 route entries. This footprint excludes
caller-owned keys and payloads.

| scenario | deleted records | live records | live nodes | node-store bytes | node-store bytes per live key |
|---|---:|---:|---:|---:|---:|
| Build | 0 | 100,000 | 513 | 2,101,248 bytes | 21.01 B/key |
| Delete-heavy | 73,279 | 26,721 | 167 | 684,032 bytes | 25.60 B/key |

For a favorable google/btree comparison, key bytes and payloads are also
excluded. google/btree still stores a key reference in each item: in this
benchmark that is a 16-byte `string` header plus an 8-byte `Position`. The
estimate below counts btree node structs, item backing arrays, child pointer
arrays, and those item slots, but not the key bytes referenced by the strings.
Storing only `Position` in google/btree would move key lookup into the
comparator, which has no error path and is not a good fit for this index.

| index | live records | live nodes | index bytes | index bytes per live key |
|---|---:|---:|---:|---:|
| minpatricia | 100,000 | 513 | 2,101,248 bytes | 21.01 B/key |
| google/btree estimated | 100,000 | 465 | 3,815,296 bytes | 38.15 B/key |

## Write Tradeoff

Writes are still the expected weak spot. The index uses fixed 4KB nodes so node
IDs can later translate to `mmap_base + id * 4096`, and it stores only compact
routes plus record positions inside the index. Insert/delete update the
affected route incrementally when the target node has room or can shrink in
place; they rebuild affected 4KB node route trees only when a page split,
subtree split, child merge, or direct sibling merge changes the node shape.
That is slower than a Go map write, but keeps the index compact, ordered, and
mmap-friendly.

## License

MIT
