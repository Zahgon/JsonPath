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

package main

import (
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	const doc = `{"a":[1,2.5,3000000000,12345678901234567890,0.12345678901234567890],"b":{"c":"x/y"}}`
	tests := []struct {
		args       []string
		doc        string
		out, err   string
		wantStatus int
	}{
		{[]string{"read", "$.b.c"}, doc, "\"x/y\"\n", "", 0},
		{[]string{"--typed", "read", "$.a"}, doc,
			"[int:1,double:2.5,long:3000000000,bigint:12345678901234567890,bigdec:0.12345678901234567890]\n", "", 0},
		{[]string{"read", "$.a.sum()"}, `{"a":[1,2]}`, "3.0\n", "", 0},
		{[]string{"read", "$.zz"}, doc, "", "PathNotFoundException: No results for path: $['zz']\n", 1},
		{[]string{"--option", "SUPPRESS_EXCEPTIONS", "read", "$.zz"}, doc, "null\n", "", 0},
		{[]string{"--option", "AS_PATH_LIST", "read", "$.a[0,1]"}, doc, "[\"$['a'][0]\",\"$['a'][1]\"]\n", "", 0},
		{[]string{"set", "$.b.c", "1"}, `{"b":{"c":0}}`, "{\"b\":{\"c\":1}}\n{\"b\":{\"c\":1}}\n", "", 0},
		{[]string{"--option", "AS_PATH_LIST", "delete", "$.a[0]"}, `{"a":[1,2]}`, "[\"$['a'][0]\"]\n{\"a\":[2]}\n", "", 0},
		{[]string{"delete", "$.zz"}, `{}`, "", "PathNotFoundException\n", 1},
		{[]string{"read", "$"}, `{"a":`, "",
			"InvalidJsonException: net.minidev.json.parser.ParseException: Unexpected End Of File position 4: EOF\n", 1},
		{[]string{"read", "$"}, `null`, "", "IllegalArgumentException: json object can not be null\n", 1},
		{[]string{"compile", "$..book[?(@.price < 10)].title"}, "", "$..['book'][?]['title']\nfalse\n", "", 0},
		{[]string{"filter", "[?(@.a == 'x' && @.b =~ /a.*/i)]"}, "", "[?(@['a'] == 'x' && @['b'] =~ /a.*/i)]\n", "", 0},
		{[]string{"--filter", "[?(@.v > 1)]", "read", "$[?].v"}, `[{"v":1},{"v":2}]`, "[2]\n", "", 0},
		{[]string{"--limit", "1", "read", "$..v"}, `[{"v":1},{"v":2}]`, "[1]\n", "", 0},
		{[]string{"read", "$[?(@.v == 'NaN')]"}, `[{"v":NaN}]`, "",
			"NumberFormatException: Character N is neither a decimal digit number, decimal point, nor \"e\" notation exponential mark.\n", 1},
		{[]string{"read", "$.a.first()"}, `{"a":[]}`, "", "internal error: IndexOutOfBoundsException\n", 3},
		{[]string{"read", "$[0].first()"}, `[[]]`, "", "PathNotFoundException: No results for path: $[0].first()\n", 1},
		{[]string{}, "", "", "usage: missing command\n", 2},
		{[]string{"read"}, "", "", "usage: read takes 1 argument\n", 2},
		{[]string{"--limit", "1", "set", "$.a", "1"}, "", "", "usage: --limit applies to read only\n", 2},
		{[]string{"--option", "NOPE", "read", "$"}, "", "", "usage: unknown option NOPE\n", 2},
		{[]string{"--nope", "read", "$"}, "", "", "usage: unknown flag --nope\n", 2},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			out, errText, status := run(tt.args, strings.NewReader(tt.doc))
			if out != tt.out || errText != tt.err || status != tt.wantStatus {
				t.Errorf("run(%q) = (%q, %q, %d), want (%q, %q, %d)", tt.args, out, errText, status, tt.out, tt.err,
					tt.wantStatus)
			}
		})
	}
}
