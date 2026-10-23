// Copyright 2011 the original author or authors (Jayway JsonPath, ported to Go).
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package java

import (
	"math"
	"strconv"
	"strings"
)

// NumberFormatError is Java's NumberFormatException; Message is its message.
type NumberFormatError struct {
	Message string
}

// Error returns the exception message.
func (e *NumberFormatError) Error() string {
	return e.Message
}

func forInputString(s string) *NumberFormatError {
	return &NumberFormatError{Message: `For input string: "` + s + `"`}
}

// ParseInt parses a decimal int32 as Java's Integer.parseInt(s): an optional sign and at least one decimal digit of
// any script, within the int range.
func ParseInt(s string) (int32, error) {
	if s == "" {
		return 0, forInputString(s)
	}
	units := ToChars(s)
	neg := false
	i := 0
	if units[0] == '-' || units[0] == '+' {
		neg = units[0] == '-'
		i = 1
		if len(units) == 1 {
			return 0, forInputString(s)
		}
	}
	var v int64
	for ; i < len(units); i++ {
		d := Digit(rune(units[i]))
		if d < 0 {
			return 0, forInputString(s)
		}
		v = v*10 + int64(d)
		if v > math.MaxInt32+1 {
			return 0, forInputString(s)
		}
	}
	if neg {
		v = -v
	}
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0, forInputString(s)
	}
	return int32(v), nil
}

// DoubleToString formats v as Java's Double.toString: the shortest decimal that reads back as v, in plain notation
// for magnitudes from 10^-3 up to 10^7 and as d.dddE±n otherwise, always with a digit after the point.
func DoubleToString(v float64) string {
	return floatToString(v, 64)
}

// FloatToString formats v as Java's Float.toString.
func FloatToString(v float32) string {
	return floatToString(float64(v), 32)
}

func floatToString(v float64, bits int) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	case v == 0:
		if math.Signbit(v) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	// Shortest round-trip digits and the decimal exponent of the first digit.
	e := strconv.FormatFloat(v, 'e', -1, bits)
	if !strings.Contains(e, ".") {
		// Java picks among two-digit decimals when one digit would do, so the closest of them wins (4.9E-324).
		e = strconv.FormatFloat(v, 'e', 1, bits)
	}
	mant, expPart, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expPart)
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	if digits == "" {
		digits = "0"
	}
	if v >= 1e-3 && v < 1e7 {
		var b strings.Builder
		b.WriteString(sign)
		if exp < 0 {
			b.WriteString("0.")
			b.WriteString(strings.Repeat("0", -exp-1))
			b.WriteString(digits)
			return b.String()
		}
		if len(digits) <= exp+1 {
			b.WriteString(digits)
			b.WriteString(strings.Repeat("0", exp+1-len(digits)))
			b.WriteString(".0")
			return b.String()
		}
		b.WriteString(digits[:exp+1])
		b.WriteByte('.')
		b.WriteString(digits[exp+1:])
		return b.String()
	}
	frac := digits[1:]
	if frac == "" {
		frac = "0"
	}
	return sign + digits[:1] + "." + frac + "E" + strconv.Itoa(exp)
}

// ParseDouble parses s as Java's Double.parseDouble for the decimal forms a JSON number token can take: an optional
// sign, digits with an optional fraction (either part may be empty, not both) and an optional exponent with at
// least one digit. Values out of range become infinities or zeros as in Java.
func ParseDouble(s string) (float64, error) {
	t := Trim(s)
	body := t
	if strings.HasPrefix(body, "-") || strings.HasPrefix(body, "+") {
		body = body[1:]
	}
	mant, exp, hasExp := strings.Cut(strings.ToLower(body), "e")
	intPart, fracPart, _ := strings.Cut(mant, ".")
	if intPart+fracPart == "" || !allASCIIDigits(intPart) || !allASCIIDigits(fracPart) || strings.Count(mant, ".") > 1 {
		return 0, forInputString(s)
	}
	if hasExp {
		e := exp
		if strings.HasPrefix(e, "-") || strings.HasPrefix(e, "+") {
			e = e[1:]
		}
		if e == "" || !allASCIIDigits(e) {
			return 0, forInputString(s)
		}
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		// Only range errors remain: ParseFloat already returns the infinity or zero Java would.
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, forInputString(s)
		}
	}
	return v, nil
}

func allASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IntValueOfDouble narrows v to int32 as Java's (int) cast: NaN becomes 0 and out-of-range values saturate.
func IntValueOfDouble(v float64) int32 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}
