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

package javaregex

import "testing"

func TestMatches(t *testing.T) {
	tests := []struct {
		pattern string
		flags   string
		input   string
		want    bool
	}{
		{"a.*", "", "abc", true},
		{"a", "", "abc", false},
		{".*REES", "i", "Nigel Rees", true},
		{"é", "i", "É", false},
		{"é", "iu", "É", true},
		{"a.b", "", "a\rb", false},
		{"a.b", "d", "a\rb", true},
		{"a.b", "s", "a\nb", true},
		{"\\s", "", "\v", true},
		{"\\w+", "", "naïve", false},
		{"\\w+", "U", "naïve", true},
		{"\\p{Lu}", "i", "é", true},
		{"\\p{Lower}", "i", "A", true},
		{"[a-z&&[^aeiou]]+", "", "bcd", true},
		{"[a-z&&[^aeiou]]+", "", "bad", false},
		{"[^a]", "i", "A", false},
		{" a b # comment", "x", "ab", true},
		{"(?i)abc", "", "ABC", true},
		{"\\Qa.b\\E", "", "a.b", true},
		{"\\Qa.b\\E", "", "axb", false},
		{"^abc$", "", "abc", true},
		{"(?<n>a)b", "", "ab", true},
	}
	for _, tt := range tests {
		re, err := Compile(tt.pattern, ParseFlags(tt.flags))
		if err != nil {
			t.Errorf("Compile(%q, %q): %v", tt.pattern, tt.flags, err)
			continue
		}
		if got := re.Matches(tt.input); got != tt.want {
			t.Errorf("/%s/%s matches %q = %v, want %v", tt.pattern, tt.flags, tt.input, got, tt.want)
		}
	}
	for _, bad := range []string{"(", "[", "a{", "*", "\\q", "[z-a]", "\\p{Nope}", "a)"} {
		if _, err := Compile(bad, 0); err == nil {
			t.Errorf("Compile(%q) succeeded, want a syntax error", bad)
		}
	}
}
