package rules

import (
	"fmt"
	"math"
	"math/bits"
)

// num is a number that stays an exact int64 while it can.
type num struct {
	i     int64
	f     float64
	isInt bool
}

func toNum(v any) (num, bool) {
	switch x := v.(type) {
	case int:
		return num{i: int64(x), f: float64(x), isInt: true}, true
	case int8:
		return num{i: int64(x), f: float64(x), isInt: true}, true
	case int16:
		return num{i: int64(x), f: float64(x), isInt: true}, true
	case int32:
		return num{i: int64(x), f: float64(x), isInt: true}, true
	case int64:
		return num{i: x, f: float64(x), isInt: true}, true
	case uint:
		return uintNum(uint64(x)), true
	case uint8:
		return uintNum(uint64(x)), true
	case uint16:
		return uintNum(uint64(x)), true
	case uint32:
		return uintNum(uint64(x)), true
	case uint64:
		return uintNum(x), true
	case float32:
		return num{f: float64(x)}, true
	case float64:
		return num{f: x}, true
	}
	return num{}, false
}

func uintNum(u uint64) num {
	if u <= math.MaxInt64 {
		return num{i: int64(u), f: float64(u), isInt: true}
	}
	return num{f: float64(u)}
}

func needNum(fn string, v any) (num, error) {
	n, ok := toNum(v)
	if !ok {
		return num{}, fmt.Errorf("%s: want a number, got %T", fn, v)
	}
	return n, nil
}

func clampFn(x, lo, hi any) (any, error) {
	a, err := needNum("clamp", x)
	if err != nil {
		return nil, err
	}
	l, err := needNum("clamp", lo)
	if err != nil {
		return nil, err
	}
	h, err := needNum("clamp", hi)
	if err != nil {
		return nil, err
	}
	if a.isInt && l.isInt && h.isInt {
		return int(min(max(a.i, l.i), h.i)), nil
	}
	return math.Min(math.Max(a.f, l.f), h.f), nil
}

func pow2ceilFn(x any) (any, error) {
	a, err := needNum("pow2ceil", x)
	if err != nil {
		return nil, err
	}
	if a.isInt && a.i <= 1<<62 {
		if a.i <= 1 {
			return 1, nil
		}
		return int(uint64(1) << bits.Len64(uint64(a.i-1))), nil
	}
	if a.f <= 1 {
		return float64(1), nil
	}
	return math.Pow(2, math.Ceil(math.Log2(a.f))), nil
}

// bytesFn is ParseBytes returning an int when the size is integral.
func bytesFn(s string) (any, error) {
	f, err := ParseBytes(s)
	if err != nil {
		return nil, err
	}
	if f == math.Trunc(f) && math.Abs(f) < 1<<62 {
		return int(f), nil
	}
	return f, nil
}
