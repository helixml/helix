package ptr

func To[T any](v T) *T {
	return &v
}

func From[T any](v *T) T {
	// If it's nil, return the zero value
	if v == nil {
		return *new(T)
	}
	return *v
}
