package orderedmap

import "testing"

func BenchmarkOrderedMapSet(b *testing.B) {
	m := NewOrderedMap[int, int](WithCapacity[int](b.N))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Set(i, i)
	}
}

func BenchmarkOrderedMapGetHit(b *testing.B) {
	const n = 1_000
	m := NewOrderedMap[int, int](WithCapacity[int](n))
	for i := 0; i < n; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get(i % n)
	}
}

func BenchmarkOrderedMapPairs(b *testing.B) {
	const n = 1_000
	m := NewOrderedMap[int, int](WithCapacity[int](n))
	for i := 0; i < n; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Pairs()
	}
}

func BenchmarkOrderedMapSortedPairs(b *testing.B) {
	const n = 1_000
	m := NewOrderedMap[int, int](
		WithCapacity[int](n),
		WithComparator[int](func(a, b int) bool { return a < b }),
	)
	for i := 0; i < n; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.SortedPairs(nil)
	}
}
