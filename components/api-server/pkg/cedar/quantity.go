package cedar

import (
	"fmt"
	"math/big"
	"strings"
)

// Kubernetes quantity suffix multipliers, expressed as exact rationals so the
// conversion from a quantity string to integer millicores or bytes is exact
// (no float rounding).
var (
	binarySuffixes = map[string]*big.Rat{
		"Ki": ratPow(1024, 1),
		"Mi": ratPow(1024, 2),
		"Gi": ratPow(1024, 3),
		"Ti": ratPow(1024, 4),
		"Pi": ratPow(1024, 5),
		"Ei": ratPow(1024, 6),
	}
	decimalSuffixes = map[string]*big.Rat{
		"n": big.NewRat(1, 1_000_000_000),
		"u": big.NewRat(1, 1_000_000),
		"m": big.NewRat(1, 1000),
		"":  big.NewRat(1, 1),
		"k": ratPow(1000, 1),
		"M": ratPow(1000, 2),
		"G": ratPow(1000, 3),
		"T": ratPow(1000, 4),
		"P": ratPow(1000, 5),
		"E": ratPow(1000, 6),
	}
)

func ratPow(base, exp int64) *big.Rat {
	r := big.NewInt(1)
	b := big.NewInt(base)
	for i := int64(0); i < exp; i++ {
		r.Mul(r, b)
	}
	return new(big.Rat).SetInt(r)
}

// parseQuantity parses a Kubernetes resource quantity (for example "500m",
// "2", "16Gi", "1G") into its value in base units (cores for CPU, bytes for
// memory) as an exact rational. It rejects empty, malformed, or negative
// quantities.
func parseQuantity(q string) (*big.Rat, error) {
	s := strings.TrimSpace(q)
	if s == "" {
		return nil, fmt.Errorf("empty quantity")
	}

	numPart, mult, err := splitSuffix(s)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Rat).SetString(numPart)
	if !ok {
		return nil, fmt.Errorf("invalid quantity %q", q)
	}
	if value.Sign() < 0 {
		return nil, fmt.Errorf("negative quantity %q", q)
	}
	return value.Mul(value, mult), nil
}

// splitSuffix separates a quantity into its numeric part and its suffix
// multiplier. Binary suffixes (two characters, ending in "i") are matched
// before single-character decimal suffixes.
func splitSuffix(s string) (string, *big.Rat, error) {
	if len(s) >= 2 {
		if mult, ok := binarySuffixes[s[len(s)-2:]]; ok {
			return s[:len(s)-2], mult, nil
		}
	}
	last := s[len(s)-1:]
	if mult, ok := decimalSuffixes[last]; ok && !isDigit(last[0]) {
		return s[:len(s)-1], mult, nil
	}
	// No recognized suffix: the whole string is numeric (possibly with an
	// exponent, which big.Rat.SetString accepts).
	return s, decimalSuffixes[""], nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// ParseMillis parses a CPU-style quantity and returns its value in millicores.
// "500m" -> 500, "2" -> 2000, "0.5" -> 500. It errors when the quantity does
// not resolve to a whole number of millicores or overflows int64.
func ParseMillis(q string) (int64, error) {
	r, err := parseQuantity(q)
	if err != nil {
		return 0, err
	}
	milli := r.Mul(r, big.NewRat(1000, 1))
	if !milli.IsInt() {
		return 0, fmt.Errorf("quantity %q does not resolve to a whole number of millicores", q)
	}
	n := milli.Num()
	if !n.IsInt64() {
		return 0, fmt.Errorf("quantity %q overflows int64 millicores", q)
	}
	return n.Int64(), nil
}

// ParseBytes parses a memory-style quantity and returns its value in bytes.
// "16Gi" -> 17179869184, "512Mi" -> 536870912, "1000" -> 1000. It errors when
// the quantity does not resolve to a whole number of bytes or overflows int64.
func ParseBytes(q string) (int64, error) {
	r, err := parseQuantity(q)
	if err != nil {
		return 0, err
	}
	if !r.IsInt() {
		return 0, fmt.Errorf("quantity %q does not resolve to a whole number of bytes", q)
	}
	n := r.Num()
	if !n.IsInt64() {
		return 0, fmt.Errorf("quantity %q overflows int64 bytes", q)
	}
	return n.Int64(), nil
}
