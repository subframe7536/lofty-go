package util

// Option represents an optional value, equivalent to Rust's `Option<T>`.
type Option[T any] struct {
	val T
	set bool
}

// Some creates an [Option] containing a value.
func Some[T any](val T) Option[T] { return Option[T]{val: val, set: true} }

// None creates an empty [Option].
func None[T any]() Option[T] { return Option[T]{} }

// Get returns the value and whether it is present.
func (o Option[T]) Get() (T, bool) { return o.val, o.set }

// IsSome reports whether the option contains a value.
func (o Option[T]) IsSome() bool { return o.set }

// IsNone reports whether the option is empty.
func (o Option[T]) IsNone() bool { return !o.set }

// OrZero returns the value if present, or the zero value.
func (o Option[T]) OrZero() T { return o.val }

// OrElse returns the value if present, or `def`.
func (o Option[T]) OrElse(def T) T {
	if o.set {
		return o.val
	}
	return def
}

// MustGet returns the value, panicking if absent.
func (o Option[T]) MustGet() T {
	if !o.set {
		panic("Option: value is None")
	}
	return o.val
}

func StringToOption(s string) Option[string] {
	if s == "" {
		return None[string]()
	}
	return Some(s)
}
