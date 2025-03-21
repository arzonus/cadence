package interval

// Stepper is an interface for stepping through an interval between two values.
// The interval is a range of integers from a minimum to a maximum value (inclusive).
type Stepper interface {
	// Next returns the next value in the interval
	Next() int

	// Previous returns the previous value in the interval
	Previous() int

	// Value returns the current value in the interval
	Value() int

	// Reset resets the stepper to its initial state
	Reset()
}
