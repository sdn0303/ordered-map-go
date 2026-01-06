// Package orderedmap provides a goroutine-safe ordered map that preserves insertion order.
//
// Design goals:
//   - O(1) lookup with insertion-order iteration.
//   - SortedPairs provides side-effect-free sorting of a snapshot.
//   - All APIs are protected by an RWMutex.
//
// See docs/ordered_map_design.md for the full rationale and trade-offs.
package orderedmap

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Pair is a key/value tuple returned by snapshot APIs such as Pairs and SortedPairs.
//
// Pair is a copy of the key/value at snapshot time. Note that if V is a reference type
// (pointer, map, slice, func, channel, etc.), the reference itself is copied.
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// ErrComparatorRequired is returned when a comparator is missing for SortedPairs:
// both the per-call comparator argument and the default comparator on the map are nil.
var ErrComparatorRequired = errors.New("comparator is required")

type orderedMapConfig[K comparable] struct {
	capacity int
	less     func(a, b K) bool
}

// Option customizes OrderedMap construction.
type Option[K comparable] func(*orderedMapConfig[K])

// WithCapacity preallocates map/slice capacity.
//
// n <= 0 is treated as "no hint" and does not change the initial capacity.
func WithCapacity[K comparable](n int) Option[K] {
	return func(cfg *orderedMapConfig[K]) {
		if n > 0 {
			cfg.capacity = n
		}
	}
}

// WithComparator sets the default comparator (less function) used by SortedPairs when
// the per-call comparator argument is nil.
func WithComparator[K comparable](less func(a, b K) bool) Option[K] {
	return func(cfg *orderedMapConfig[K]) {
		cfg.less = less
	}
}

// OrderedMap keeps insertion order while providing O(1) lookup.
//
// Semantics:
//   - Keys are unique.
//   - Set on an existing key updates the value without changing the insertion order.
//   - Pairs/Keys/Values return snapshots (copies) and never expose internal slices.
//   - SortedPairs returns a sorted snapshot and does not mutate the underlying order.
//
// Concurrency:
//   - All methods are goroutine-safe.
//   - SortedPairs sorts outside the lock so writers are not blocked by the sort.
//
// Complexity (n = number of entries):
//   - Set/Get/Has/Len: amortized O(1)
//   - Delete/InsertBefore/InsertAfter: O(n) due to slice search/compaction/shift
//   - Pairs/Keys/Values: O(n) to build a snapshot
//   - SortedPairs: O(n log n) to sort a snapshot
type OrderedMap[K comparable, V any] struct {
	mu    sync.RWMutex
	data  map[K]V
	order []K
	less  func(a, b K) bool
}

// NewOrderedMap constructs an OrderedMap with optional configuration.
func NewOrderedMap[K comparable, V any](opts ...Option[K]) *OrderedMap[K, V] {
	cfg := orderedMapConfig[K]{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &OrderedMap[K, V]{
		data:  make(map[K]V, cfg.capacity),
		order: make([]K, 0, cfg.capacity),
		less:  cfg.less,
	}
}

// Set inserts or updates a value.
//
// If key does not exist, it is appended to the end of the insertion order.
// If key exists, only the value is updated and the insertion order is unchanged.
func (m *OrderedMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; exists {
		m.data[key] = value
		return
	}

	m.data[key] = value
	m.order = append(m.order, key)
}

// Upsert inserts a new key with newVal or updates an existing key using fn.
//
// If key does not exist, newVal is stored and key is appended to the insertion order
// (fn is ignored and may be nil).
// If key exists, fn must be non-nil and is executed under the map's write lock to provide
// an atomic read-modify-write update.
//
// Callback contract:
//   - fn must be fast and side-effect-free.
//   - fn MUST NOT call back into the same OrderedMap (Get/Set/Delete/...); that will deadlock.
//
// Returns the resulting value, a flag indicating insertion, and an error if fn is nil for updates.
func (m *OrderedMap[K, V]) Upsert(key K, newVal V, fn func(old V) V) (V, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if old, exists := m.data[key]; exists {
		if fn == nil {
			var zero V
			return zero, false, fmt.Errorf("upsert requires fn when key exists: %v", key)
		}
		updated := fn(old)
		m.data[key] = updated
		return updated, false, nil
	}

	m.data[key] = newVal
	m.order = append(m.order, key)
	return newVal, true, nil
}

// Get retrieves a value by key.
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

// Has reports whether the key exists.
func (m *OrderedMap[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.data[key]
	return exists
}

// Len returns the number of elements.
func (m *OrderedMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Delete removes a key if present and returns the value.
//
// Note: Delete is O(n) because it compacts the insertion-order slice.
func (m *OrderedMap[K, V]) Delete(key K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	val, exists := m.data[key]
	if !exists {
		var zero V
		return zero, false
	}

	delete(m.data, key)
	m.removeFromOrder(key)
	return val, true
}

// Clear removes all entries.
//
// Clear preserves internal capacity for reuse. To release memory, allocate a new OrderedMap.
func (m *OrderedMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[K]V, cap(m.order))
	m.order = m.order[:0]
}

// Pairs returns a snapshot of key/value pairs in insertion order.
//
// The returned slice is independent from the map; callers may modify it freely.
func (m *OrderedMap[K, V]) Pairs() []Pair[K, V] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pairs := make([]Pair[K, V], 0, len(m.order))
	for _, k := range m.order {
		pairs = append(pairs, Pair[K, V]{Key: k, Value: m.data[k]})
	}
	return pairs
}

// Keys returns a snapshot of keys in insertion order.
//
// The returned slice is independent from the map; callers may modify it freely.
func (m *OrderedMap[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]K(nil), m.order...)
}

// Values returns a snapshot of values in insertion order.
//
// The returned slice is independent from the map; callers may modify it freely.
func (m *OrderedMap[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values := make([]V, 0, len(m.order))
	for _, k := range m.order {
		values = append(values, m.data[k])
	}
	return values
}

// SortedPairs returns a snapshot sorted by the provided comparator or the default comparator.
// It does not mutate the underlying order.
//
// Comparator selection:
//   - If less != nil, it is used.
//   - Otherwise, the default comparator set by WithComparator is used.
//   - If both are nil, ErrComparatorRequired is returned.
//
// Concurrency:
//   - Snapshot creation happens under a read lock.
//   - Sorting and comparator calls happen outside the lock so writers are not blocked.
//
// Ordering:
//   - Sorting is stable. Keys that are considered equal by the comparator keep their
//     insertion-relative order (i.e., the snapshot order).
//
// The comparator must define a strict weak ordering. If it is inconsistent, the result is undefined.
func (m *OrderedMap[K, V]) SortedPairs(less func(a, b K) bool) ([]Pair[K, V], error) {
	m.mu.RLock()
	if less == nil {
		less = m.less
	}
	if less == nil {
		m.mu.RUnlock()
		return nil, ErrComparatorRequired
	}

	pairs := make([]Pair[K, V], 0, len(m.order))
	for _, k := range m.order {
		pairs = append(pairs, Pair[K, V]{Key: k, Value: m.data[k]})
	}
	m.mu.RUnlock()

	// Sort outside the lock to avoid blocking writers.
	// Use stable sort so keys considered equal keep their insertion-relative order.
	sort.SliceStable(pairs, func(i, j int) bool {
		return less(pairs[i].Key, pairs[j].Key)
	})
	return pairs, nil
}

// InsertBefore inserts key/value before the target key.
//
// Returns an error if:
//   - target does not exist, or
//   - key already exists.
func (m *OrderedMap[K, V]) InsertBefore(target K, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; exists {
		return fmt.Errorf("key already exists: %v", key)
	}

	idx := m.indexOf(target)
	if idx == -1 {
		return fmt.Errorf("target key not found: %v", target)
	}

	m.data[key] = value
	m.order = append(m.order, key)       // extend
	copy(m.order[idx+1:], m.order[idx:]) // shift right
	m.order[idx] = key
	return nil
}

// InsertAfter inserts key/value after the target key.
//
// Returns an error if:
//   - target does not exist, or
//   - key already exists.
func (m *OrderedMap[K, V]) InsertAfter(target K, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; exists {
		return fmt.Errorf("key already exists: %v", key)
	}

	idx := m.indexOf(target)
	if idx == -1 {
		return fmt.Errorf("target key not found: %v", target)
	}

	insertPos := idx + 1
	m.data[key] = value
	m.order = append(m.order, key)
	copy(m.order[insertPos+1:], m.order[insertPos:])
	m.order[insertPos] = key
	return nil
}

func (m *OrderedMap[K, V]) indexOf(key K) int {
	for i, k := range m.order {
		if k == key {
			return i
		}
	}
	return -1
}

func (m *OrderedMap[K, V]) removeFromOrder(key K) {
	for i, k := range m.order {
		if k == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}
