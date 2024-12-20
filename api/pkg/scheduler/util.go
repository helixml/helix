package scheduler

import "github.com/puzpuzpuz/xsync/v3"

// Values returns a slice of all values in the map.
func Values[K, V comparable](m *xsync.MapOf[K, V]) []V {
	values := make([]V, 0, m.Size())
	// nolint:revive
	m.Range(func(key K, value V) bool {
		values = append(values, value)
		return true
	})
	return values
}

func ValuesMap[K, V comparable](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, value := range m {
		values = append(values, value)
	}
	return values
}

func Keys[K comparable, V any](m *xsync.MapOf[K, V]) []K {
	keys := make([]K, 0, m.Size())
	// Collect all map keys.
	m.Range(func(key K, _ V) bool {
		keys = append(keys, key)
		return true
	})
	return keys
}

func KeysMap[K, V comparable](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// Filter filters a slice based on a predicate function and returns a new slice with matching elements.
func Filter[v any](s []v, f func(v) bool) []v {
	vals := make([]v, 0, len(s))
	// Iterate through the slice and append elements that satisfy the predicate.
	for _, value := range s {
		if f(value) {
			vals = append(vals, value)
		}
	}
	return vals
}

// FilterMap filters a map based on a predicate function and returns a new map with matching entries.
func FilterMap[k comparable, v any](m map[k]v, f func(v) bool) map[k]v {
	newMap := make(map[k]v, len(m))
	// Iterate through the map and copy entries that satisfy the predicate.
	for k, v := range m {
		if f(v) {
			newMap[k] = v
		}
	}
	return newMap
}

// Reverse reverses the order of elements in a slice.
func Reverse[v any](s []v) []v {
	// Create a new slice with the same capacity as the original.
	reversed := make([]v, len(s))
	// Copy elements in reverse order.
	for i, j := 0, len(s)-1; i < len(s); i, j = i+1, j-1 {
		reversed[i] = s[j]
	}
	return reversed
}
