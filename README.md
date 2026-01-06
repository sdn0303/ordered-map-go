# OrderedMap

A generic ordered map for Go 1.24+ that preserves insertion order while providing O(1) lookups.
All methods are protected by an RWMutex, and SortedPairs accepts a comparator to return a sorted
snapshot without side effects.

## Features

- Preserves insertion order while providing O(1) lookups like a built-in map.
- Keys are always unique; calling Set again does not change the order.
- SortedPairs accepts a comparator and returns a sorted snapshot (sorting happens **outside the lock**).
- SortedPairs uses a stable sort: keys considered equal keep their insertion (snapshot) order.
- All APIs are goroutine-safe (RWMutex + snapshot returns).
- See `docs/ordered_map_design.md` for design details.

## Example API

```go
// Constructor
m := NewOrderedMap[string, int](WithComparator(func(a, b string) bool { return a < b }))

// Basic operations
m.Set("b", 20)
m.Set("a", 10)
v, ok := m.Get("a")
_ = ok

// Insertion-order access
pairs := m.Pairs() // [{Key:"b",Value:20}, {Key:"a",Value:10}]

// Sorted access (side-effect-free)
sorted, err := m.SortedPairs(nil) // if nil, uses the default less registered via WithComparator
_ = err
_ = sorted
```

## Status

- This repository contains the design documentation and an implementation.
- The legacy `ExtendedMap` implementation is being replaced by `ordered_map.go`.

## Complexity & intended size (misuse prevention)

- **O(1)**: `Set` / `Get` / `Has` / `Len`
- **O(n)**: `Delete` / `InsertBefore` / `InsertAfter` (search/compaction/shift on the order slice)
- **O(n log n)**: `SortedPairs`
- **Intended size**: small-to-medium ordered data where order is part of the spec (roughly tens to hundreds of entries).
  Not suitable for huge datasets or hot-path caches.
- **Clear policy**: `Clear()` keeps internal capacity for reuse. To aggressively release memory, create a new map with
  `NewOrderedMap`.

## Callback contract (important)

- `SortedPairs` comparator: invoked outside the lock after snapshotting. A heavy comparator will make the call slower,
  but it does not block writers.
- `Upsert` update function `fn`: invoked **under the write lock** to provide atomic updates. `fn` must be fast and
  side-effect-free, and MUST NOT re-enter the same `OrderedMap` (calling `Get/Set/Delete/...` will deadlock).

## Intended use cases

- Managing evaluation order for configs/rules
- Enumerating feature flag definitions
- LLM prompt pipelines where order is part of the spec

## Design docs

- `docs/ordered_map_design.md`
