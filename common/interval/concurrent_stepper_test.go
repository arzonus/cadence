package interval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcurrentStepper_Next(t *testing.T) {
	s := NewLinearStepper(10, 20, 3)
	c := NewConcurrentStepper(s)
	assert.Equal(t, 15, c.Next())
}

func TestConcurrentStepper_Previous(t *testing.T) {
	s := NewLinearStepper(10, 20, 3)
	c := NewConcurrentStepper(s)

	// move cursor to max value
	c.Next()
	c.Next()

	assert.Equal(t, 15, c.Previous())
}

func TestConcurrentStepper_Reset(t *testing.T) {
	s := NewLinearStepper(10, 20, 3)
	c := NewConcurrentStepper(s)

	c.Next()
	c.Reset()

	assert.Equal(t, 10, c.Value())
}

func TestConcurrentStepper_Value(t *testing.T) {
	s := NewLinearStepper(10, 20, 3)
	c := NewConcurrentStepper(s)
	c.Next()

	assert.Equal(t, 15, c.Value())
}
