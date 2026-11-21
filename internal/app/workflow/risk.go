package workflow

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// Weight is one weight of risk.weights: the factor of the files matching Glob in the risk score of the diff.
type Weight struct {
	Glob   string
	Factor float64
}

// Valid tells why a weight is not valid: an empty glob, or a factor that is negative or not finite.
func (w Weight) Valid() error {
	switch {
	case strings.TrimSpace(w.Glob) == "":
		return errors.New("weight: empty glob")
	case math.IsNaN(w.Factor) || math.IsInf(w.Factor, 0) || w.Factor < 0:
		return fmt.Errorf("weight %q: the factor must be a finite, non-negative number", w.Glob)
	}
	return nil
}
