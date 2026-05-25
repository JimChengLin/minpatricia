package minpatricia

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"testing"

	"github.com/google/btree"
)

const benchBTreeDegree = (MaxNodeReps + 1) / 2

type benchSize struct {
	name string
	n    int
}

var quickBenchSizes = []benchSize{
	{name: "1K", n: 1_000},
	{name: "10K", n: 10_000},
}

var largeBenchSizes = []benchSize{
	{name: "100K", n: 100_000},
}

func benchSizes() []benchSize {
	if os.Getenv("MIN_PATRICIA_BENCH_LARGE") == "1" {
		sizes := make([]benchSize, 0, len(quickBenchSizes)+len(largeBenchSizes))
		sizes = append(sizes, quickBenchSizes...)
		sizes = append(sizes, largeBenchSizes...)
		return sizes
	}
	return quickBenchSizes
}

func mutationBenchSizes() []benchSize {
	if os.Getenv("MIN_PATRICIA_BENCH_LARGE") == "1" {
		sizes := make([]benchSize, 0, 1+len(largeBenchSizes))
		sizes = append(sizes, quickBenchSizes[len(quickBenchSizes)-1])
		sizes = append(sizes, largeBenchSizes...)
		return sizes
	}
	return quickBenchSizes[len(quickBenchSizes)-1:]
}

type benchData struct {
	keys     [][]byte
	strings  []string
	records  *HeapRecordStore[struct{}]
	position []Position
}

func BenchmarkGet(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/go_map", func(b *testing.B) {
			m := buildGoMap(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				sink = m[data.strings[i%len(data.strings)]]
			}
			_ = sink
		})

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				item, ok := tree.Get(benchBTreeItem{key: data.strings[i%len(data.strings)]})
				if !ok {
					b.Fatalf("Get failed: key=%q", data.strings[i%len(data.strings)])
				}
				sink = item.pos
			}
			_ = sink
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				pos, ok, err := idx.Get(data.keys[i%len(data.keys)])
				if err != nil || !ok {
					b.Fatalf("Get failed: pos=%d ok=%v err=%v", pos, ok, err)
				}
				sink = pos
			}
			_ = sink
		})
	}
}

func BenchmarkPutReplace(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)
		replacements := newReplacementPositions(data)

		b.Run(size.name+"/go_map", func(b *testing.B) {
			m := buildGoMap(data)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				j := i % len(data.strings)
				m[data.strings[j]] = replacements[j]
			}
		})

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				j := i % len(data.strings)
				tree.ReplaceOrInsert(benchBTreeItem{
					key: data.strings[j],
					pos: replacements[j],
				})
			}
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				j := i % len(data.keys)
				if _, _, err := idx.Put(data.keys[j], replacements[j]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPutInsert(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/go_map", func(b *testing.B) {
			benchmarkPerItem(b, len(data.strings), func() {
				m := make(map[string]Position, len(data.strings))
				for j, key := range data.strings {
					m[key] = data.position[j]
				}
				if len(m) != len(data.strings) {
					b.Fatalf("len=%d want=%d", len(m), len(data.strings))
				}
			})
		})

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			benchmarkPerItem(b, len(data.strings), func() {
				tree := btree.NewG(benchBTreeDegree, benchBTreeLess)
				for j, key := range data.strings {
					tree.ReplaceOrInsert(benchBTreeItem{
						key: key,
						pos: data.position[j],
					})
				}
				if tree.Len() != len(data.strings) {
					b.Fatalf("len=%d want=%d", tree.Len(), len(data.strings))
				}
			})
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			benchmarkPerItem(b, len(data.keys), func() {
				idx := NewWithRecords(data.records)
				for j, key := range data.keys {
					if _, _, err := idx.Put(key, data.position[j]); err != nil {
						b.Fatal(err)
					}
				}
				if idx.Len() != len(data.keys) {
					b.Fatalf("len=%d want=%d", idx.Len(), len(data.keys))
				}
			})
		})
	}
}

func BenchmarkVisitOrdered(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/go_map_sort_keys", func(b *testing.B) {
			m := buildGoMap(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				keys := make([]string, 0, len(m))
				for key := range m {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					sink = m[key]
				}
			}
			_ = sink
		})

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				tree.Ascend(func(item benchBTreeItem) bool {
					sink = item.pos
					return true
				})
			}
			_ = sink
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				if err := idx.Visit(func(_ []byte, pos Position) bool {
					sink = pos
					return true
				}); err != nil {
					b.Fatal(err)
				}
			}
			_ = sink
		})
	}
}

func BenchmarkSeek(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				target := data.strings[i%len(data.strings)]
				found := false
				tree.AscendGreaterOrEqual(benchBTreeItem{key: target}, func(item benchBTreeItem) bool {
					sink = item.pos
					found = true
					return false
				})
				if !found {
					b.Fatalf("Seek(%q) failed", target)
				}
			}
			_ = sink
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				target := data.keys[i%len(data.keys)]
				found := false
				if err := idx.AscendGreaterOrEqual(target, func(_ []byte, pos Position) bool {
					sink = pos
					found = true
					return false
				}); err != nil {
					b.Fatal(err)
				}
				if !found {
					b.Fatalf("Seek(%q) failed", target)
				}
			}
			_ = sink
		})
	}
}

func BenchmarkReverseSeek(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				target := data.strings[i%len(data.strings)]
				found := false
				tree.DescendLessOrEqual(benchBTreeItem{key: target}, func(item benchBTreeItem) bool {
					sink = item.pos
					found = true
					return false
				})
				if !found {
					b.Fatalf("ReverseSeek(%q) failed", target)
				}
			}
			_ = sink
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			b.ReportAllocs()
			b.ResetTimer()

			var sink Position
			for i := 0; i < b.N; i++ {
				target := data.keys[i%len(data.keys)]
				found := false
				if err := idx.DescendLessOrEqual(target, func(_ []byte, pos Position) bool {
					sink = pos
					found = true
					return false
				}); err != nil {
					b.Fatal(err)
				}
				if !found {
					b.Fatalf("ReverseSeek(%q) failed", target)
				}
			}
			_ = sink
		})
	}
}

func BenchmarkDeleteHeavy(b *testing.B) {
	for _, size := range mutationBenchSizes() {
		data := newBenchData(size.n)
		positions := deleteHeavyPositions(b, data)
		keys := make([][]byte, len(positions))
		strings := make([]string, len(positions))
		for i, pos := range positions {
			key, ok := data.records.Key(pos)
			if !ok {
				b.Fatalf("missing key at position %d", pos)
			}
			keys[i] = key
			strings[i] = string(key)
		}

		b.Run(size.name+"/go_map", func(b *testing.B) {
			m := buildGoMap(data)
			next := 0
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if next == len(strings) {
					b.StopTimer()
					m = buildGoMap(data)
					next = 0
					b.StartTimer()
				}
				delete(m, strings[next])
				next++
			}
		})

		b.Run(size.name+"/google_btree", func(b *testing.B) {
			tree := buildBTree(data)
			next := 0
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if next == len(strings) {
					b.StopTimer()
					tree = buildBTree(data)
					next = 0
					b.StartTimer()
				}
				if _, ok := tree.Delete(benchBTreeItem{key: strings[next]}); !ok {
					b.Fatalf("Delete(%q) failed", strings[next])
				}
				next++
			}
		})

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			idx := buildPatricia(b, data)
			next := 0
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if next == len(keys) {
					b.StopTimer()
					idx = buildPatricia(b, data)
					next = 0
					b.StartTimer()
				}
				if _, deleted, err := idx.Delete(keys[next]); err != nil || !deleted {
					b.Fatalf("Delete(%q) deleted=%v err=%v", keys[next], deleted, err)
				}
				next++
			}
		})
	}
}

func BenchmarkFootprint(b *testing.B) {
	for _, size := range benchSizes() {
		data := newBenchData(size.n)

		b.Run(size.name+"/min_patricia", func(b *testing.B) {
			var liveNodes int
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := buildPatricia(b, data)
				liveNodes = idx.LiveNodes()
			}
			b.StopTimer()
			b.ReportMetric(float64(liveNodes), "nodes/op")
			b.ReportMetric(float64(liveNodes*NodeSize)/float64(size.n), "node_B/key")
		})
	}
}

func benchmarkPerItem(b *testing.B, items int, runBatch func()) {
	b.Helper()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBatch()
	}
	b.StopTimer()
	runtime.ReadMemStats(&after)

	totalItems := float64(b.N) * float64(items)
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/totalItems, "ns/op")
	b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc)/totalItems, "B/op")
	b.ReportMetric(float64(after.Mallocs-before.Mallocs)/totalItems, "allocs/op")
}

func newBenchData(n int) benchData {
	rng := rand.New(rand.NewSource(int64(n)))
	records := NewHeapRecordStore[struct{}]()
	keys := make([][]byte, 0, n)
	strings := make([]string, 0, n)
	positions := make([]Position, 0, n)
	seen := make(map[string]struct{}, n)

	for len(keys) < n {
		key := fmt.Sprintf("key-%08x-%08x", rng.Uint32(), rng.Uint32())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		keyBytes := []byte(key)
		pos := records.Add(keyBytes, struct{}{})
		keys = append(keys, keyBytes)
		strings = append(strings, key)
		positions = append(positions, pos)
	}

	return benchData{
		keys:     keys,
		strings:  strings,
		records:  records,
		position: positions,
	}
}

func newReplacementPositions(data benchData) []Position {
	replacements := make([]Position, len(data.keys))
	for i, key := range data.keys {
		pos := data.records.Add(key, struct{}{})
		replacements[i] = pos
	}
	return replacements
}

func buildGoMap(data benchData) map[string]Position {
	m := make(map[string]Position, len(data.strings))
	for i, key := range data.strings {
		m[key] = data.position[i]
	}
	return m
}

type benchBTreeItem struct {
	key string
	pos Position
}

func benchBTreeLess(a, b benchBTreeItem) bool {
	return a.key < b.key
}

func buildBTree(data benchData) *btree.BTreeG[benchBTreeItem] {
	tree := btree.NewG(benchBTreeDegree, benchBTreeLess)
	for i, key := range data.strings {
		tree.ReplaceOrInsert(benchBTreeItem{
			key: key,
			pos: data.position[i],
		})
	}
	return tree
}

func buildPatricia(tb testing.TB, data benchData) *Index {
	tb.Helper()

	return buildPatriciaWithNodes(tb, data, NewHeapNodeStore())
}

func buildPatriciaWithNodes(tb testing.TB, data benchData, nodes NodeStore) *Index {
	tb.Helper()

	idx := NewWithNodes(data.records, nodes)
	for i, key := range data.keys {
		if _, _, err := idx.Put(key, data.position[i]); err != nil {
			tb.Fatal(err)
		}
	}
	return idx
}

func deleteHeavyPositions(tb testing.TB, data benchData) []Position {
	tb.Helper()

	idx := buildPatricia(tb, data)
	var positions []Position
	if err := idx.collectDeleteHeavyPositions(idx.root(), &positions); err != nil {
		tb.Fatal(err)
	}
	if len(positions) == 0 {
		tb.Fatalf("no delete-heavy candidates for %d keys", len(data.keys))
	}
	return positions
}

func (idx *Index) collectDeleteHeavyPositions(n *node, positions *[]Position) error {
	size := int(n.size)
	for i := 0; i < size; i++ {
		r := n.reps[i]
		if !r.isChild() {
			continue
		}

		child, err := idx.nodeByID(r.childID())
		if err != nil {
			return err
		}
		childSize := int(child.size)
		safeDeletes := size + childSize - MaxNodeReps - 2
		if safeDeletes > 0 {
			added := 0
			for j := 1; j+1 < childSize && added < safeDeletes; j++ {
				r := child.reps[j]
				if r.isChild() {
					continue
				}
				*positions = append(*positions, r.position())
				added++
			}
		}
		if err := idx.collectDeleteHeavyPositions(child, positions); err != nil {
			return err
		}
	}
	return nil
}
