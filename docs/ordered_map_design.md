# OrderedMap Design Document

## Purpose & assumptions

- Go does not provide an ordered map in the standard library, so we provide one with explicit semantics.
- Target: Go 1.24+ generics; type-safe and goroutine-safe.
- Primary use cases: small datasets where order matters (configs, flag definitions, LLM prompt stages, etc.).

## Design goals

- Provide O(1) lookups while preserving insertion order.
- Keys are always unique. Setting an existing key overwrites the value but does not re-insert or change order.
- Sorting is side-effect-free. A comparator is injected by the caller and we return a snapshot.
- All APIs are protected by an RWMutex (parallel reads, exclusive writes).

## Data structure

```go
type OrderedMap[K comparable, V any] struct {
    mu    sync.RWMutex      // thread-safe reads/writes
    data  map[K]V           // O(1) lookup
    order []K               // preserves insertion order (compacted on delete)
    less  func(a, b K) bool // optional comparator used by SortedPairs
}
```

## Proposed API

- Constructor
  - `NewOrderedMap[K, V](opts ...Option) *OrderedMap[K, V]`
  - Options: `WithComparator(func(a, b K) bool)`, `WithCapacity(n int)`
- Basic operations
  - `Set(key K, val V)`: append when new; overwrite value when existing without changing order.
  - `Get(key K) (V, bool)`
  - `Has(key K) bool`
  - `Delete(key K) (V, bool)`: remove from the map and from the order slice.
  - `Len() int`, `Clear()`
- Insertion-order access
  - `Pairs() []Pair[K, V]`: insertion-order snapshot (copy).
  - `Keys() []K`, `Values() []V`: insertion-order snapshots.
- Sorted access (side-effect-free)
  - `SortedPairs(less func(a, b K) bool) ([]Pair[K, V], error)`: per-call comparator takes precedence; if nil uses the
    stored less; if both nil returns a comparator error.
- Additional operations (optional)
  - `InsertBefore(target K, key K, val V) error`
  - `InsertAfter(target K, key K, val V) error`
  - `Upsert(key K, newVal V, f func(old V) V)`: update an existing key via a function.

## Behavioral details

- Calling Set on a duplicate key does not append to `order` (insertion order is invariant).
- Delete compacts `order` in O(n). Document that large datasets are not recommended.
- Pairs/Keys/Values return copies, so post-call mutations do not race with internal state.
- SortedPairs sorts a snapshot and does not mutate the underlying order.
- SortedPairs uses a stable sort: keys considered equal by the comparator keep insertion-relative order.
- Upsert update function `f` is executed under the write lock to make the update atomic. `f` must be fast,
  side-effect-free, and MUST NOT re-enter the same OrderedMap (Get/Set/Delete/...).

## Concurrency policy

- Every method acquires `mu`: read methods use RLock/RUnlock; write methods use Lock/Unlock.
- To avoid long lock holds, SortedPairs snapshots under the read lock and sorts outside the lock.
- Returned slices are copies; modifying them does not affect the map.
- SortedPairs comparator is called outside the lock (does not block writers). A slow comparator only slows the caller.

## Error handling

- SortedPairs returns an error when no comparator is available, instead of panicking.
- InsertBefore/After return an error if the target is missing.

## Complexity

- Set/Has/Get: O(1)
- Delete: O(n) (compacting the order slice)
- Pairs/Keys/Values: O(n) (building a snapshot)
- SortedPairs: O(n log n) (sorting a snapshot)

## Example use cases

- Managing evaluation order for configs and rules.
- Enumerating and tracking feature flag definitions.
- LLM prompt pipelines: store stages as `OrderedMap[string, PromptStage]` and combine them in insertion order at build time.

## Non-goals

- Huge datasets or hot-path cache use (due to O(n) deletes).
- Maintaining sorted order at all times via tree structures (insertion-time sort) — out of scope.

## Implementation guidance (next steps)

- File: implement in `ordered_map.go`, replacing the old `extended_map.go`.
- Update go.mod to Go 1.24 and align the module name with the repository.
- Use an option pattern for flexible constructors.
- Validate concurrency via table-driven tests and `go test -race ./...`.

## Test plan

- Verify CRUD and insertion-order preservation.
- Ensure Set overwrites do not change order.
- SortedPairs: comparator presence, stable behavior across comparators, missing-comparator error.
- Delete compaction and Len consistency.
- Reuse after Clear (no leaks).
- Concurrency: mix Set/Get/Delete across goroutines and validate with -race.
