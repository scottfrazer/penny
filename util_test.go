package main

import (
	"fmt"
	"math"
	"testing"
)

type FVTest struct {
	Rate              float64
	Nper              float64
	Pmt               float64
	PV                float64
	BeginningOfPeriod bool
	Expected          float64
}

func TestFV(t *testing.T) {
	tests := []FVTest{
		{.04, 30, 0, 100000, false, -324339.75},
		{.04, 30, 10000, 100000, false, -885189.13},
		{.04, 30, -10000, -100000, false, 885189.13},
		{.04, 30, 10000, -100000, false, -236509.63},
	}
	round := func(x float64) float64 {
		return math.Floor(x/100) * 100
	}

	for _, tc := range tests {
		name := fmt.Sprintf("FV(%.2f,%.2f,%.2f,%.2f,%v)", tc.Rate, tc.Nper, tc.Pmt, tc.PV, tc.BeginningOfPeriod)
		t.Run(name, func(t *testing.T) {
			actual := FV(tc.Rate, tc.Nper, tc.Pmt, tc.PV, tc.BeginningOfPeriod)
			if round(actual) != round(tc.Expected) {
				t.Logf("Expecting %f, got %f", tc.Expected, actual)
				t.Fail()
			}
		})
	}
}
