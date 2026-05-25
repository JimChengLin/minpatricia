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

Apple M1 Pro, Go 1.22, 100K random keys, `-benchtime=500ms`.

| operation | go map | google/btree | minpatricia |
|---|---:|---:|---:|
| Get | 15.98 ns/op | 236.2 ns/op | 153.7 ns/op |
| Seek >= | - | 270.2 ns/op | 187.0 ns/op |
| Seek <= | - | 287.3 ns/op | 187.8 ns/op |
| Replace | 29.99 ns/op | 291.7 ns/op | 178.0 ns/op |
| Build insert, per key | 22.03 ns/op | 318.6 ns/op | 5928 ns/op |
| Delete-heavy | 60.54 ns/op | 216.1 ns/op | 1504 ns/op |

Node-store footprint for the same 100K-key benchmark:

| metric | value |
|---|---:|
| node size | 4096 bytes |
| max reps per node | 339 |
| live nodes | 513 |
| node-store bytes | 2,101,248 bytes |
| node-store bytes per key | 21.01 B/key |

This footprint excludes caller-owned keys and payloads.

## Write Tradeoff

Writes are the expected weak spot. The index uses fixed 4KB nodes so node IDs
can later translate to `mmap_base + id * 4096`, and it stores only compact
routes plus record positions inside the index. Insert/delete can therefore pay
for rebuilding a 4KB node route tree and occasionally splitting or merging
nodes. That is slower than a Go map write, but keeps the index compact,
ordered, and mmap-friendly.
