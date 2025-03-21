package interval

import "sync"

// ConcurrentStepper is a thread-safe stepper that wraps another stepper.
type ConcurrentStepper struct {
	mx      sync.RWMutex
	stepper Stepper
}

// NewConcurrentStepper wraps the given stepper in a thread-safe stepper.
func NewConcurrentStepper(stepper Stepper) *ConcurrentStepper {
	return &ConcurrentStepper{stepper: stepper}
}

// Next returns the next value in the interval.
func (c *ConcurrentStepper) Next() int {
	c.mx.Lock()
	defer c.mx.Unlock()

	return c.stepper.Next()
}

// Previous returns the previous value in the interval.
func (c *ConcurrentStepper) Previous() int {
	c.mx.Lock()
	defer c.mx.Unlock()

	return c.stepper.Previous()
}

// Value returns the current value in the interval.
func (c *ConcurrentStepper) Value() int {
	c.mx.RLock()
	defer c.mx.RUnlock()

	return c.stepper.Value()
}

// Reset resets the interval to its initial state.
func (c *ConcurrentStepper) Reset() {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.stepper.Reset()
}
