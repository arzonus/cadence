package interval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLinearStepper(t *testing.T) {
	for name, c := range map[string]struct {
		min, max  int
		stepCount int
		want      *LinearStepper
	}{
		"min greater max, stepCount greater interval": {
			min:       10,
			max:       1,
			stepCount: 10, // interval: 10 - 1 = 9
			want: &LinearStepper{
				cur:   0,
				steps: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
		},
		"max greater min, stepCount is one": {
			min:       0,
			max:       10,
			stepCount: 1,
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 10},
			},
		},
		"max greater min, stepCount is two": {
			min:       0,
			max:       10,
			stepCount: 2,
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 10},
			},
		},
		"max greater min, stepCount is three": {
			min:       0,
			max:       10,
			stepCount: 3,
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 5, 10},
			},
		},
		"max greater min, stepCount is four": {
			min:       0,
			max:       10,
			stepCount: 4,
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 3, 6, 10},
			},
		},
		"max greater min, stepCount is six": {
			min:       0,
			max:       10,
			stepCount: 6,
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 2, 4, 6, 8, 10},
			},
		},
		"max greater min than 1, stepCount is greater interval": {
			min:       9,
			max:       10,
			stepCount: 10,
			want: &LinearStepper{
				cur:   0,
				steps: []int{9, 10},
			},
		},
		"max equal mim, stepCount is greater interval": {
			min:       10,
			max:       10,
			stepCount: 10,
			want: &LinearStepper{
				cur:   0,
				steps: []int{10, 10},
			},
		},
		"max greater min, interval is less than stepCount": {
			min:       0,
			max:       10,
			stepCount: 20, // interval: 10 - 0 = 10
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
		},
		"max greater min, interval is equal to stepCount": {
			min:       0,
			max:       10,
			stepCount: 10, // interval: 10 - 0 = 10
			want: &LinearStepper{
				cur:   0,
				steps: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 10},
			},
		},
		"negative max and min, stepCount is two": {
			min:       -1000 * 1000,
			max:       -1000,
			stepCount: 35,
			want: &LinearStepper{
				cur:   0,
				steps: []int{-1000000, -970618, -941236, -911853, -882471, -853089, -823706, -794324, -764942, -735559, -706177, -676795, -647412, -618030, -588648, -559265, -529883, -500500, -471118, -441736, -412353, -382971, -353589, -324206, -294824, -265442, -236059, -206677, -177295, -147912, -118530, -89148, -59765, -30383, -1000},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := NewLinearStepper(c.min, c.max, c.stepCount)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestLinearStepper_Next(t *testing.T) {
	for name, c := range map[string]struct {
		cur   int
		steps []int

		wantValue int
		wantCur   int
	}{
		"cur equals to min": {
			cur:   0,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 1,
			wantCur:   1,
		},
		"cur equals to middle": {
			cur:   3, // value - 3
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 4,
			wantCur:   4,
		},
		"cur equals to max": {
			cur:   5,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 5,
			wantCur:   5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &LinearStepper{
				cur:   c.cur,
				steps: c.steps,
			}
			assert.Equal(t, c.wantValue, s.Next())
			assert.Equal(t, c.wantCur, s.cur)
		})
	}
}

func TestLinearStepper_Previous(t *testing.T) {
	for name, c := range map[string]struct {
		cur   int
		steps []int

		wantValue int
		wantCur   int
	}{
		"cur equals to min": {
			cur:   0,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 0,
			wantCur:   0,
		},
		"cur equals to middle": {
			cur:   3, // value - 3
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 2,
			wantCur:   2,
		},
		"cur equals to max": {
			cur:   6,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 5,
			wantCur:   5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &LinearStepper{
				cur:   c.cur,
				steps: c.steps,
			}
			assert.Equal(t, c.wantValue, s.Previous())
			assert.Equal(t, c.wantCur, s.cur)
		})
	}
}

func TestLinearStepper_Reset(t *testing.T) {
	for name, c := range map[string]struct {
		cur   int
		steps []int

		wantValue int
		wantCur   int
	}{
		"cur equals to min": {
			cur:   0,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 0,
			wantCur:   0,
		},
		"cur equals to middle": {
			cur:   3, // value - 3
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 0,
			wantCur:   0,
		},
		"cur equals to max": {
			cur:   6,
			steps: []int{0, 1, 2, 3, 4, 5},

			wantValue: 0,
			wantCur:   0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &LinearStepper{
				cur:   c.cur,
				steps: c.steps,
			}

			s.Reset()
			assert.Equal(t, c.wantValue, s.Value())
			assert.Equal(t, c.wantCur, s.cur)
		})
	}
}

func TestLinearStepper_Value(t *testing.T) {
	stepper := &LinearStepper{
		cur:   3,
		steps: []int{0, 1, 2, 3, 4, 5},
	}
	assert.Equal(t, 3, stepper.Value())
}
