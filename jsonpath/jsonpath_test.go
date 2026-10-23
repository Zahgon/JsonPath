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

package jsonpath

import (
	"errors"
	"testing"

	"github.com/json-path/JsonPath/smartjson"
)

const storeJSON = `{"store":{"book":[
{"category":"reference","author":"Nigel Rees","title":"Sayings of the Century","price":8.95},
{"category":"fiction","author":"Evelyn Waugh","title":"Sword of Honour","price":12.99},
{"category":"fiction","author":"Herman Melville","title":"Moby Dick","isbn":"0-553-21311-3","price":8.99},
{"category":"fiction","author":"J. R. R. Tolkien","title":"The Lord of the Rings","isbn":"0-395-19395-8","price":22.99}],
"bicycle":{"color":"red","price":19.95}},"expensive":10}`

func mustParse(t *testing.T, text string) any {
	t.Helper()
	doc, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return doc
}

func TestCompile(t *testing.T) {
	tests := []struct {
		path, want string
		definite   bool
	}{
		{"$.store.book[0].title", "$['store']['book'][0]['title']", true},
		{"store.book", "$['store']['book']", true},
		{"$..book[?(@.price < 10)].title", "$..['book'][?]['title']", false},
		{"$['a','b']", "$['a','b']", true},
		{"$['a','b'].c", "$['a','b']['c']", false},
		{"$[1:3]", "$[1:3]", false},
		{"$[0,1]", "$[0,1]", false},
		{"$.*", "$[*]", false},
		{"$..book.length()", "$.length()", true},
		{"$.sum(1, $.a)", "$.sum(...)", true},
	}
	for _, tt := range tests {
		p, err := Compile(tt.path)
		if err != nil {
			t.Errorf("Compile(%q): %v", tt.path, err)
			continue
		}
		if p.String() != tt.want || p.IsDefinite() != tt.definite {
			t.Errorf("Compile(%q) = %s definite %v, want %s definite %v", tt.path, p, p.IsDefinite(), tt.want, tt.definite)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct{ path, want string }{
		{"$.", "InvalidPathException: Path must not end with a '.' or '..'"},
		{"$.a b", "InvalidPathException: Use bracket notion ['my prop'] if your property contains blank characters. position: 2"},
		{"$['a'", "InvalidPathException: Property must be separated by comma or Property must be terminated close square bracket at index 4"},
		{"$[-]", "InvalidPathException: Failed to parse token in ArrayIndexOperation: -"},
		{"$[1:-]", `InvalidPathException: java.lang.NumberFormatException: For input string: "-"`},
		{"$[?]", "InvalidPathException: Not enough predicates supplied for filter [?] at position 1"},
		{"$.sum(1", "InvalidPathException: Arguments to function: 'sum' are not closed properly."},
		{"", "IllegalArgumentException: json can not be null or empty"},
	}
	for _, tt := range tests {
		_, err := Compile(tt.path)
		if err == nil || err.Error() != tt.want {
			t.Errorf("Compile(%q) error = %v, want %q", tt.path, err, tt.want)
		}
	}
}

func TestRead(t *testing.T) {
	tests := []struct {
		path    string
		options []Option
		want    string
	}{
		{"$.store.book[*].author", nil, `["Nigel Rees","Evelyn Waugh","Herman Melville","J. R. R. Tolkien"]`},
		{"$..book[2].title", nil, `["Moby Dick"]`},
		{"$.store.book[-1].price", nil, `22.99`},
		{"$..book[?(@.isbn)].title", nil, `["Moby Dick","The Lord of the Rings"]`},
		{"$..book[?(@.price <= $['expensive'])].price", nil, `[8.95,8.99]`},
		{"$..book[?(@.author =~ /.*REES/i)].title", nil, `["Sayings of the Century"]`},
		{"$..book.length()", nil, `4`},
		{"$..price.max()", nil, `22.99`},
		{"$.store.bicycle.color", []Option{AlwaysReturnList}, `["red"]`},
		{"$.store.book[0,1].price", []Option{AsPathList}, `["$['store']['book'][0]['price']","$['store']['book'][1]['price']"]`},
		{"$.store.book[0].isbn", []Option{DefaultPathLeafToNull}, `null`},
		{"$.store.nope.x", []Option{SuppressExceptions}, `null`},
		{"$.store.book[*].isbn", nil, `["0-553-21311-3","0-395-19395-8"]`},
	}
	for _, tt := range tests {
		p, err := Compile(tt.path)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.path, err)
		}
		got, err := p.Read(mustParse(t, storeJSON), NewConfiguration(tt.options...))
		if err != nil {
			t.Errorf("Read(%q, %v): %v", tt.path, tt.options, err)
			continue
		}
		if s := smartjson.Marshal(got, smartjson.StyleCompact); s != tt.want {
			t.Errorf("Read(%q, %v) = %s, want %s", tt.path, tt.options, s, tt.want)
		}
	}
}

func TestReadErrors(t *testing.T) {
	doc := mustParse(t, storeJSON)
	p, _ := Compile("$.store.book[*].isbn")
	if _, err := p.Read(doc, NewConfiguration(RequireProperties)); !errors.Is(err, ErrPathNotFound) ||
		err.Error() != "PathNotFoundException: No results for path: $['store']['book'][0]['isbn']" {
		t.Errorf("Read with RequireProperties error = %v", err)
	}
	p, _ = Compile("$.store.nope.x")
	_, err := p.Read(doc, DefaultConfiguration())
	if !errors.Is(err, ErrInvalidPath) || !errors.Is(err, ErrJSONPath) || errors.Is(err, ErrInvalidJSON) {
		t.Errorf("a missing property must be a PathNotFound (and InvalidPath) error, got %v", err)
	}
	p, _ = Compile("$.store.book.length()")
	if _, err := p.Read(doc, NewConfiguration(AsPathList)); err == nil ||
		err.Error() != "JsonPathException: Options AS_PATH_LIST and ALWAYS_RETURN_LIST are not allowed when using path functions!" {
		t.Errorf("function path with AsPathList error = %v", err)
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		op   func(p *JSONPath, doc any) (any, error)
		path string
		want string
	}{
		{func(p *JSONPath, doc any) (any, error) { return p.Set(doc, int32(1), DefaultConfiguration()) }, "$.a[*]", `{"a":[1,1],"b":{"c":1}}`},
		{func(p *JSONPath, doc any) (any, error) { return p.Delete(doc, DefaultConfiguration()) }, "$.a[0]", `{"a":[2],"b":{"c":1}}`},
		{func(p *JSONPath, doc any) (any, error) { return p.Add(doc, "x", DefaultConfiguration()) }, "$.a", `{"a":[1,2,"x"],"b":{"c":1}}`},
		{func(p *JSONPath, doc any) (any, error) { return p.Put(doc, "d", nil, DefaultConfiguration()) }, "$.b", `{"a":[1,2],"b":{"c":1,"d":null}}`},
		{func(p *JSONPath, doc any) (any, error) { return p.RenameKey(doc, "c", "e", DefaultConfiguration()) }, "$.b", `{"a":[1,2],"b":{"e":1}}`},
	}
	for _, tt := range tests {
		doc := mustParse(t, `{"a":[1,2],"b":{"c":1}}`)
		p, err := Compile(tt.path)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.path, err)
		}
		got, err := tt.op(p, doc)
		if err != nil {
			t.Errorf("write at %q: %v", tt.path, err)
			continue
		}
		if s := smartjson.Marshal(got, smartjson.StyleCompact); s != tt.want {
			t.Errorf("write at %q = %s, want %s", tt.path, s, tt.want)
		}
	}
	p, _ := Compile("$")
	if _, err := p.Set(mustParse(t, `{}`), int32(1), DefaultConfiguration()); !errors.Is(err, ErrInvalidModification) {
		t.Errorf("setting the root error = %v, want an invalid modification", err)
	}
}

func TestDocumentContext(t *testing.T) {
	ctx, err := ParseDocument(storeJSON, DefaultConfiguration())
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	got, err := ctx.Limit(2).Read("$..price")
	if err != nil || smartjson.Marshal(got, smartjson.StyleCompact) != `[8.95,12.99]` {
		t.Errorf("Limit(2).Read($..price) = %v, %v", got, err)
	}
	filter, err := ParseFilter("[?(@.price > 20)]")
	if err != nil {
		t.Fatalf("ParseFilter: %v", err)
	}
	got, err = ctx.Read("$.store.book[?].title", filter)
	if err != nil || smartjson.Marshal(got, smartjson.StyleCompact) != `["The Lord of the Rings"]` {
		t.Errorf("Read with a filter = %v, %v", got, err)
	}
	if _, err := ctx.Set("$.store.bicycle.color", "blue"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, _ := ctx.Read("$.store.bicycle.color"); got != "blue" {
		t.Errorf("after Set the color is %v, want blue", got)
	}
	var seen []string
	listening := &DocumentContext{json: ctx.JSON(), conf: DefaultConfiguration().WithListeners(func(r FoundResult) EvaluationContinuation {
		seen = append(seen, r.Path)
		return Continue
	})}
	if _, err := listening.Read("$.store.book[0,1].price"); err != nil || len(seen) != 2 {
		t.Errorf("listener saw %v, %v", seen, err)
	}
}

func TestFilterString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"[?(@.a == 1 && @.b)]", "[?(@['a'] == 1 && @['b'])]"},
		{"[?(!(@.a == 1))]", "[?(@['a'] == 1 ! null)]"},
		{"[?(@.a in ['x', \"y\"])]", `[?(@['a'] IN ['x', "y"])]`},
		{"[?(@.a == 1e5)]", "[?(@['a'] == 1E+5)]"},
		{"[?(@.a =~ /x/i)]", "[?(@['a'] =~ /x/i)]"},
	}
	for _, tt := range tests {
		f, err := ParseFilter(tt.in)
		if err != nil {
			t.Errorf("ParseFilter(%q): %v", tt.in, err)
			continue
		}
		if f.String() != tt.want {
			t.Errorf("ParseFilter(%q) = %s, want %s", tt.in, f, tt.want)
		}
	}
}
