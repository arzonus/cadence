package interval

// LinearStepper is a stepper that steps through an interval linearly with a fixed step size.
// The fixed step size is calculated based on the interval and the number of steps provided.
type LinearStepper struct {
	cur   int
	steps []int
}

// NewLinearStepper returns a new LinearStepper.
// The interval is a range of integers from min to max. The stepCount is the number of steps between min and max (inclusive).
// If stepCount is less or equal than 2 or the difference between min and max is less than 1, the stepper will only have min and max.
//   - interval [0, 10], stepCount 1 -> [0, 10]
//   - interval [10, 10], stepCount 2 -> [10, 10]
//
// If the difference between min and max is less than stepCount, the stepper will have all the values between min and max.
//   - interval [0, 5], stepCount 10 -> [0, 1, 2, 3, 4, 5]
//   - interval [0, 10], stepCount 11 -> [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
func NewLinearStepper(min, max, stepCount int) *LinearStepper {
	// Ensure min <= max
	if min > max {
		min, max = max, min
	}

	totalInterval := max - min

	// If stepCount is less than 2 or the difference between min and max is less than 1,
	// return a stepper with only min and max
	if stepCount <= 2 || totalInterval <= 1 {
		return &LinearStepper{
			cur:   0,
			steps: []int{min, max},
		}
	}

	// if the total interval is less than the step count, return a stepper with all the values
	if totalInterval < stepCount {
		steps := make([]int, totalInterval+1)

		steps[0], steps[totalInterval] = min, max

		for i := 1; i < totalInterval; i++ {
			steps[i] = min + i
		}

		return &LinearStepper{
			cur:   0,
			steps: steps,
		}
	}

	// Calculate step size as a float to ensure even distribution
	stepSize := float64(max-min) / float64(stepCount-1)

	// Generate steps
	steps := make([]int, stepCount)
	for i := 0; i < stepCount; i++ {
		steps[i] = min + int(float64(i)*stepSize)
	}

	// Ensure the last value is exactly max (to avoid floating-point errors)
	steps[stepCount-1] = max

	return &LinearStepper{
		steps: steps,
		cur:   0,
	}
}

// Next returns the next value in the interval.
func (s *LinearStepper) Next() int {
	if s.cur < len(s.steps)-1 {
		s.cur++
	}
	return s.steps[s.cur]
}

// Previous returns the previous value in the interval.
func (s *LinearStepper) Previous() int {
	if s.cur > 0 {
		s.cur--
	}
	return s.steps[s.cur]
}

// Value returns the current value in the interval.
func (s *LinearStepper) Value() int {
	return s.steps[s.cur]
}

// Reset resets the stepper to the beginning of the interval.
func (s *LinearStepper) Reset() {
	s.cur = 0
}
