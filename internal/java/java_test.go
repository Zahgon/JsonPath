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
	"testing"
)

func TestDoubleToString(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.0"}, {math.Copysign(0, -1), "-0.0"}, {1, "1.0"}, {0.5, "0.5"}, {100, "100.0"}, {1e7, "1.0E7"},
		{123456789, "1.23456789E8"}, {9999999.5, "9999999.5"}, {0.001, "0.001"}, {0.0001, "1.0E-4"}, {1e23, "1.0E23"},
		{4.9e-324, "4.9E-324"}, {math.MaxFloat64, "1.7976931348623157E308"}, {-2.5e-10, "-2.5E-10"},
		{math.Inf(1), "Infinity"}, {math.Inf(-1), "-Infinity"}, {math.NaN(), "NaN"}, {math.Nextafter(0.3, 1), "0.30000000000000004"},
	}
	for _, tt := range tests {
		if got := DoubleToString(tt.in); got != tt.want {
			t.Errorf("DoubleToString(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := FloatToString(float32(math.NaN())); got != "NaN" {
		t.Errorf("FloatToString(NaN) = %q, want NaN", got)
	}
	if got := FloatToString(1.4e-45); got != "1.4E-45" {
		t.Errorf("FloatToString(1.4e-45) = %q, want 1.4E-45", got)
	}
}

func TestDecimal(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1e5", "1E+5"}, {"0.0000001", "1E-7"}, {"-1.20E-3", "-0.00120"}, {"١٢", "12"}, {"10.50", "10.50"}, {"-0", "0"},
		{"1.", "1"}, {".5", "0.5"}, {"+3", "3"}, {"123.456e2", "12345.6"}, {"1E+2", "1E+2"},
	}
	for _, tt := range tests {
		d, err := ParseDecimal(tt.in)
		if err != nil {
			t.Errorf("ParseDecimal(%q) failed: %v", tt.in, err)
			continue
		}
		if got := d.String(); got != tt.want {
			t.Errorf("ParseDecimal(%q).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
	for _, bad := range []string{"", ".", "1e", "e5", "1.2.3", "Infinity", "1_0", " 1"} {
		if _, err := ParseDecimal(bad); err == nil {
			t.Errorf("ParseDecimal(%q) succeeded, want a NumberFormatError", bad)
		}
	}
	a, _ := ParseDecimal("1.0")
	b, _ := ParseDecimal("1.000")
	if a.Cmp(b) != 0 {
		t.Errorf("1.0 and 1.000 compare as %d, want 0", a.Cmp(b))
	}
	big, _ := ParseDecimal("4294967297.9")
	if got := big.IntValue(); got != 1 {
		t.Errorf("IntValue(4294967297.9) = %d, want 1", got)
	}
}

func TestParseInt(t *testing.T) {
	for in, want := range map[string]int32{"0": 0, "-12": -12, "+7": 7, "٣": 3, "2147483647": 2147483647} {
		got, err := ParseInt(in)
		if err != nil || got != want {
			t.Errorf("ParseInt(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-", "+", "1 ", "2147483648", "1.0"} {
		_, err := ParseInt(in)
		if err == nil || err.Error() != `For input string: "`+in+`"` {
			t.Errorf("ParseInt(%q) error = %v, want the Java NumberFormatException message", in, err)
		}
	}
}

func TestStrings(t *testing.T) {
	if got := Len("a😀"); got != 3 {
		t.Errorf("Len(a😀) = %d, want 3", got)
	}
	if Compare("￿", "😀") <= 0 {
		t.Error("Compare must order U+FFFF after a surrogate pair, as Java does")
	}
	if got := Trim("\t x \n"); got != "x" {
		t.Errorf("Trim = %q, want x", got)
	}
	if IsWhitespace(' ') || !IsWhitespace(' ') || !IsWhitespace(0x1f) {
		t.Error("IsWhitespace does not follow Character.isWhitespace")
	}
	if Digit('٣') != 3 || Digit('x') != -1 {
		t.Error("Digit does not follow Character.digit")
	}
}
