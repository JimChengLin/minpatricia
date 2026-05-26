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
The table reports the 100K-key rows from the full benchmark suite. Set
`MIN_PATRICIA_BENCH_ONLY=minpatricia` to skip the go map and google/btree
comparison rows during local minpatricia-only runs.

| operation | go map | google/btree | minpatricia |
|---|---:|---:|---:|
| Get | 15.81 ns/op | 242.4 ns/op | 128.3 ns/op |
| Seek >= | - | 253.4 ns/op | 153.0 ns/op |
| Seek <= | - | 257.9 ns/op | 147.2 ns/op |
| Replace | 21.63 ns/op | 253.7 ns/op | 134.3 ns/op |
| Build insert, per key | 21.00 ns/op | 295.5 ns/op | 512.1 ns/op |
| Visit FullSet Ordered | 16,834,654 ns/op | 237,255 ns/op | 886,110 ns/op |
| Visit FullSet Reverse | 21,514,207 ns/op | 256,490 ns/op | 887,777 ns/op |
| Delete-heavy | 59.08 ns/op | 202.7 ns/op | 135.9 ns/op |

Node-store footprint for the same 100K-key benchmark. Node size is 4096
bytes and each node can hold up to 339 route entries. This footprint excludes
caller-owned keys and payloads.

| scenario | deleted records | live records | live nodes | node-store bytes | node-store bytes per live key |
|---|---:|---:|---:|---:|---:|
| Build | 0 | 100,000 | 515 | 2,109,440 bytes | 21.09 B/key |
| Delete-heavy | 32,412 | 67,588 | 508 | 2,080,768 bytes | 30.79 B/key |

For a favorable google/btree comparison, key bytes and payloads are also
excluded. google/btree still stores a key reference in each item: in this
benchmark that is a 16-byte `string` header plus an 8-byte `Position`. The
estimate below counts btree node structs, item backing arrays, child pointer
arrays, and those item slots, but not the key bytes referenced by the strings.
Storing only `Position` in google/btree would move key lookup into the
comparator, which has no error path and is not a good fit for this index.

| index | live records | live nodes | index bytes | index bytes per live key |
|---|---:|---:|---:|---:|
| minpatricia | 100,000 | 515 | 2,109,440 bytes | 21.09 B/key |
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
