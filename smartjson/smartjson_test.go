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

package smartjson

import (
	"math/big"
	"testing"
)

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		in        string
		className string
		text      string
	}{
		{"1", "java.lang.Integer", "1"},
		{"2147483648", "java.lang.Long", "2147483648"},
		{"9223372036854775808", "java.math.BigInteger", "9223372036854775808"},
		{"-", "java.lang.Integer", "0"},
		{"007", "java.lang.Integer", "7"},
		{"1.5", "java.lang.Double", "1.5"},
		{"1e400", "java.lang.Double", "Infinity"},
		{"1.0000000000000000E10", "java.math.BigDecimal", "10000000000.000000"},
		{"1.0000000000000000E30", "java.math.BigDecimal", "1.0000000000000000E+30"},
		{"NaN", "java.lang.Float", "NaN"},
		{"1e", "java.lang.String", "1e"},
		{"12 34", "java.lang.String", "12 34"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if ClassName(v) != tt.className || ToString(v) != tt.text {
			t.Errorf("Parse(%q) = %s %s, want %s %s", tt.in, ClassName(v), ToString(v), tt.className, tt.text)
		}
	}
}

func TestParsePermissive(t *testing.T) {
	tests := []struct{ in, want string }{
		{"{a:b c}", `{"a":"b c"}`},
		{"{'a':'b'}", `{"a":"b"}`},
		{"[,1,,2,]", `[1,2]`},
		{`{"a":1,"a":2,"b":3}`, `{"a":2,"b":3}`},
		{`{"a":1} trailing`, `{"a":1}`},
		{`["\x41\q"]`, `["A"]`},
		{"", `""`},
		{"[12é]", `[12,"é"]`},
	}
	for _, tt := range tests {
		v, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got := Marshal(v, StyleCompact); got != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"a" 1}`, "Unexpected character (1) at position 4."},
		{`[1}`, "Unexpected End Of File position 2: EOF"},
		{`{"a":1,"b"}`, "Unexpected character (}) at position 9."},
		{`{"a":[}`, "Unexpected character (}) at position 6."},
		{`["abc`, "Unexpected End Of File position 5: null"},
		{`["\u12G4"]`, "Unexpected unicode escape sequence G at position 6."},
	}
	for _, tt := range tests {
		_, err := Parse(tt.in)
		if err == nil || err.Error() != tt.want {
			t.Errorf("Parse(%q) error = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func TestMarshal(t *testing.T) {
	obj := NewObject()
	obj.Put("s", "a/b €\u0080\"\\")
	obj.Put("n", big.NewInt(5))
	obj.Put("d", 1e10)
	obj.Put("f", float32(1.5))
	obj.Put("arr", NewArray(nil, true, int64(3)))
	want := "{\"s\":\"a/b\\u2028\\u20AC\\u0080\\\"\\\\\",\"n\":5,\"d\":1.0E10,\"f\":1.5,\"arr\":[null,true,3]}"
	if got := Marshal(obj, StyleCompact); got != want {
		t.Errorf("Marshal compact = %s, want %s", got, want)
	}
	if got, want := Quote("a/b", StyleWeb), `"a\/b"`; got != want {
		t.Errorf("Quote web = %s, want %s", got, want)
	}
	if got, want := ToString(obj), `{s=a/b`+" "+`€`+"\u0080"+`"\, n=5, d=1.0E10, f=1.5, arr=[null,true,3]}`; got != want {
		t.Errorf("ToString = %s, want %s", got, want)
	}
}
