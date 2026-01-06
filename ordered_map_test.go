package orderedmap

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestSetGetHasAndOrder(t *testing.T) {
	m := NewOrderedMap[string, int]()

	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("a", 3) // update should not change order

	if !m.Has("a") || !m.Has("b") {
		t.Fatalf("Has failed")
	}
	if v, _ := m.Get("a"); v != 3 {
		t.Fatalf("Get returned %v, want 3", v)
	}
	if m.Len() != 2 {
		t.Fatalf("Len=%d, want 2", m.Len())
	}

	pairs := m.Pairs()
	want := []Pair[string, int]{{Key: "b", Value: 2}, {Key: "a", Value: 3}}
	if !reflect.DeepEqual(pairs, want) {
		t.Fatalf("Pairs=%v, want %v", pairs, want)
	}
}

func TestDeleteAndClear(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)

	if v, ok := m.Delete("a"); !ok || v != 1 {
		t.Fatalf("Delete failed, v=%v ok=%v", v, ok)
	}
	if m.Has("a") || m.Len() != 1 {
		t.Fatalf("State after delete invalid")
	}

	m.Clear()
	if m.Len() != 0 || len(m.Pairs()) != 0 {
		t.Fatalf("Clear failed")
	}
}

func TestPairsSnapshot(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)

	pairs := m.Pairs()
	m.Set("c", 3)

	if len(pairs) != 2 {
		t.Fatalf("snapshot length affected by later mutations")
	}
}

func TestSortedPairs(t *testing.T) {
	m := NewOrderedMap[string, int](WithComparator(func(a, b string) bool { return a < b }))
	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("c", 3)

	sorted, err := m.SortedPairs(nil)
	if err != nil {
		t.Fatalf("SortedPairs returned error: %v", err)
	}
	want := []Pair[string, int]{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
		{Key: "c", Value: 3},
	}
	if !reflect.DeepEqual(sorted, want) {
		t.Fatalf("SortedPairs=%v, want %v", sorted, want)
	}

	// override comparator
	desc, err := m.SortedPairs(func(a, b string) bool { return a > b })
	if err != nil {
		t.Fatalf("SortedPairs override error: %v", err)
	}
	if desc[0].Key != "c" || desc[2].Key != "a" {
		t.Fatalf("custom comparator not applied: %v", desc)
	}
}

func TestSortedPairsMissingComparator(t *testing.T) {
	m := NewOrderedMap[int, int]()
	m.Set(1, 1)
	if _, err := m.SortedPairs(nil); err == nil {
		t.Fatalf("expected comparator error")
	} else if !errors.Is(err, ErrComparatorRequired) {
		t.Fatalf("error=%v, want ErrComparatorRequired", err)
	}
}

func TestSortedPairsStableWhenEqual(t *testing.T) {
	// Sort by length; keys with equal length must keep insertion order.
	m := NewOrderedMap[string, int](WithComparator(func(a, b string) bool { return len(a) < len(b) }))
	m.Set("aa", 1)
	m.Set("bb", 2)
	m.Set("c", 3)

	sorted, err := m.SortedPairs(nil)
	if err != nil {
		t.Fatalf("SortedPairs returned error: %v", err)
	}
	gotKeys := []string{sorted[0].Key, sorted[1].Key, sorted[2].Key}
	wantKeys := []string{"c", "aa", "bb"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("SortedPairs stable order keys=%v, want %v", gotKeys, wantKeys)
	}
}

func TestInsertBeforeAfter(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("c", 3)

	if err := m.InsertBefore("c", "b", 2); err != nil {
		t.Fatalf("InsertBefore error: %v", err)
	}
	if err := m.InsertAfter("c", "d", 4); err != nil {
		t.Fatalf("InsertAfter error: %v", err)
	}

	gotOrder := m.Keys()
	wantOrder := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("order=%v, want %v", gotOrder, wantOrder)
	}

	if err := m.InsertBefore("x", "z", 0); err == nil {
		t.Fatalf("expected error for missing target")
	}
	if err := m.InsertAfter("a", "b", 99); err == nil {
		t.Fatalf("expected error for duplicate key")
	}
}

func TestUpsert(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)

	v, inserted, err := m.Upsert("a", 10, func(old int) int { return old + 1 })
	if err != nil {
		t.Fatalf("Upsert update error: %v", err)
	}
	if inserted || v != 2 {
		t.Fatalf("Upsert update result v=%v inserted=%v", v, inserted)
	}

	v2, inserted2, err := m.Upsert("b", 5, func(old int) int { return old }) // fn ignored on insert
	if err != nil {
		t.Fatalf("Upsert insert error: %v", err)
	}
	if !inserted2 || v2 != 5 || m.Len() != 2 {
		t.Fatalf("Upsert insert result v=%v inserted=%v len=%d", v2, inserted2, m.Len())
	}

	if _, _, err := m.Upsert("a", 0, nil); err == nil {
		t.Fatalf("expected error when fn is nil for existing key")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewOrderedMap[int, int]()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			m.Set(v, v)
			m.Get(v)
			m.Has(v)
		}(i)
	}
	wg.Wait()

	if m.Len() != 50 {
		t.Fatalf("Len=%d, want 50", m.Len())
	}
}

func TestKeysSnapshotIndependence(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)

	keys := m.Keys()
	keys[0] = "x"

	if m.Has("x") {
		t.Fatalf("modifying Keys() result should not affect map state")
	}
	want := []string{"a", "b"}
	if got := m.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys=%v, want %v", got, want)
	}
}

func TestPairsSnapshotIndependence(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)

	pairs := m.Pairs()
	pairs[0].Value = 999

	if v, _ := m.Get("a"); v != 1 {
		t.Fatalf("modifying Pairs() result should not affect map values, got %v want 1", v)
	}
}

func TestDeleteCompactsOrder(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	m.Delete("b")
	if got, want := m.Keys(), []string{"a", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys after delete=%v, want %v", got, want)
	}

	m.Delete("a")
	if got, want := m.Keys(), []string{"c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys after delete=%v, want %v", got, want)
	}
}
