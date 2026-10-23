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
	"math/big"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/internal/javaregex"
	"github.com/json-path/JsonPath/smartjson"
)

// valueNode is an operand of a filter expression: a literal, a path, or a value a path evaluated to.
type valueNode interface {
	String() string
	// equals is the operand equality the comparison operators use; it is not always symmetric.
	equals(other valueNode) bool
}

type (
	patternNode struct {
		pattern string
		flags   string
		re      *javaregex.Regexp
	}
	// jsonNode is JSON text from the filter (parsed on use) or an array or object from the document.
	jsonNode struct {
		text   string
		value  any
		parsed bool
	}
	stringNode struct {
		value       string
		singleQuote bool
	}
	// numberNode holds a decimal; a nil number is the not-a-number result of converting an unparsable string.
	numberNode struct {
		number *java.Decimal
	}
	booleanNode   struct{ value bool }
	nullNode      struct{}
	undefinedNode struct{}
	valueListNode struct{ nodes []valueNode }
	pathNode      struct {
		path        *compiledPath
		existsCheck bool
		shouldExist bool
	}
)

var (
	nullValue        = &nullNode{}
	trueValue        = &booleanNode{value: true}
	falseValue       = &booleanNode{value: false}
	undefinedOperand = &undefinedNode{}
	notANumber       = &numberNode{}
)

func boolNode(b bool) *booleanNode {
	if b {
		return trueValue
	}
	return falseValue
}

func newPatternNode(literal string) (*patternNode, error) {
	begin := strings.IndexByte(literal, '/')
	end := strings.LastIndexByte(literal, '/')
	pattern := literal[begin+1 : end]
	flags := literal[end+1:]
	re, err := javaregex.Compile(pattern, javaregex.ParseFlags(flags))
	if err != nil {
		return nil, newError(KindIllegalArgument, "%s", err.Error())
	}
	return &patternNode{pattern: pattern, flags: flags, re: re}, nil
}

// String renders the operand as JsonPath prints it in a filter.
func (n *patternNode) String() string {
	if !strings.HasPrefix(n.pattern, "/") {
		return "/" + n.pattern + "/" + n.flags
	}
	return n.pattern
}

func (n *patternNode) equals(o valueNode) bool {
	return valueNode(n) == o
}

// String renders the operand as JsonPath prints it in a filter.
func (n *jsonNode) String() string {
	if n.parsed {
		return smartjson.ToString(n.value)
	}
	return n.text
}

// parse returns the node's JSON value, parsing filter text anew each time as JsonPath does.
func (n *jsonNode) parse() (any, error) {
	if n.parsed {
		return n.value, nil
	}
	v, err := smartjson.Parse(n.text)
	if err != nil {
		return nil, wrap(KindIllegalArgument, err)
	}
	return v, nil
}

func (n *jsonNode) isArray() (bool, error) {
	v, err := n.parse()
	return isArray(v), err
}

func (n *jsonNode) length() (int, error) {
	v, err := n.parse()
	if err != nil {
		return 0, err
	}
	if arr, ok := v.(*smartjson.Array); ok {
		return arr.Len(), nil
	}
	return -1, nil
}

func (n *jsonNode) isEmpty() (bool, error) {
	v, err := n.parse()
	if err != nil {
		return false, err
	}
	switch t := v.(type) {
	case *smartjson.Array:
		return t.Len() == 0, nil
	case *smartjson.Object:
		// JsonPath casts the value to a collection here, which an object is not.
		return false, classCast(t)
	case string:
		return t == "", nil
	}
	return true, nil
}

// asValueListNode converts an array into a list of operands, or returns the undefined node.
func (n *jsonNode) asValueListNode() (valueNode, error) {
	v, err := n.parse()
	if err != nil {
		return nil, err
	}
	arr, ok := v.(*smartjson.Array)
	if !ok {
		return undefinedOperand, nil
	}
	list := &valueListNode{}
	for _, e := range arr.Elems() {
		node, err := toValueNode(e)
		if err != nil {
			return nil, err
		}
		list.nodes = append(list.nodes, node)
	}
	return list, nil
}

// equalsJSON compares the node's own value with the other node's parsed value.
func (n *jsonNode) equalsJSON(o *jsonNode) (bool, error) {
	if n == o {
		return true, nil
	}
	other, err := o.parse()
	if err != nil {
		return false, err
	}
	if n.parsed {
		return javaEquals(n.value, other), nil
	}
	s, ok := other.(string)
	return ok && s == n.text, nil
}

func (n *jsonNode) equals(o valueNode) bool {
	other, ok := o.(*jsonNode)
	if !ok {
		return false
	}
	if n == other {
		return true
	}
	if n.parsed != other.parsed {
		return false
	}
	if n.parsed {
		return javaEquals(n.value, other.value)
	}
	return n.text == other.text
}

func newStringNode(literal string, escape bool) (*stringNode, error) {
	units := java.ToChars(literal)
	if !escape || len(units) <= 1 {
		return &stringNode{value: literal, singleQuote: true}, nil
	}
	open, close := units[0], units[len(units)-1]
	singleQuote := true
	switch {
	case open == '\'' && close == '\'':
		units = units[1 : len(units)-1]
	case open == '"' && close == '"':
		units = units[1 : len(units)-1]
		singleQuote = false
	}
	s, err := unescape(units.String())
	if err != nil {
		return nil, err
	}
	return &stringNode{value: s, singleQuote: singleQuote}, nil
}

// String renders the operand as JsonPath prints it in a filter.
func (n *stringNode) String() string {
	quote := "'"
	if !n.singleQuote {
		quote = "\""
	}
	return quote + escapeString(n.value) + quote
}

func (n *stringNode) asNumberNode() *numberNode {
	d, err := java.ParseDecimal(n.value)
	if err != nil {
		return notANumber
	}
	return &numberNode{number: &d}
}

func (n *stringNode) equals(o valueNode) bool {
	switch t := o.(type) {
	case *stringNode:
		return n.value == t.value
	case *numberNode:
		return n.value == t.number.String()
	}
	return false
}

func newNumberNode(text string) (*numberNode, error) {
	d, err := java.ParseDecimal(text)
	if err != nil {
		return nil, asError(err)
	}
	return &numberNode{number: &d}, nil
}

// String renders the operand as JsonPath prints it in a filter.
func (n *numberNode) String() string {
	return n.number.String()
}

func (n *numberNode) equals(o valueNode) bool {
	var that *numberNode
	switch t := o.(type) {
	case *numberNode:
		that = t
	case *stringNode:
		that = t.asNumberNode()
	default:
		return false
	}
	if n == that {
		return true
	}
	if that == notANumber {
		return false
	}
	return n.number.Cmp(*that.number) == 0
}

// String renders the operand as JsonPath prints it in a filter.
func (n *booleanNode) String() string {
	if n.value {
		return "true"
	}
	return "false"
}

func (n *booleanNode) equals(o valueNode) bool {
	b, ok := o.(*booleanNode)
	return ok && b.value == n.value
}

// String renders the operand as JsonPath prints it in a filter.
func (*nullNode) String() string { return "null" }

func (*nullNode) equals(o valueNode) bool {
	_, ok := o.(*nullNode)
	return ok
}

// String renders the operand as JsonPath prints it in a filter.
func (*undefinedNode) String() string {
	return "com.jayway.jsonpath.internal.filter.ValueNodes$UndefinedNode"
}

func (*undefinedNode) equals(valueNode) bool { return false }

// String renders the operand as JsonPath prints it in a filter.
func (n *valueListNode) String() string {
	parts := make([]string, len(n.nodes))
	for i, node := range n.nodes {
		parts[i] = node.String()
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (n *valueListNode) contains(node valueNode) bool {
	for _, e := range n.nodes {
		if node.equals(e) {
			return true
		}
	}
	return false
}

func (n *valueListNode) subsetOf(right *valueListNode) bool {
	for _, left := range n.nodes {
		if !right.contains(left) {
			return false
		}
	}
	return true
}

func (n *valueListNode) equals(o valueNode) bool {
	other, ok := o.(*valueListNode)
	if !ok {
		return false
	}
	if len(n.nodes) != len(other.nodes) {
		return false
	}
	for i := range n.nodes {
		if !n.nodes[i].equals(other.nodes[i]) {
			return false
		}
	}
	return true
}

func newPathNode(text string, existsCheck, shouldExist bool) (*pathNode, error) {
	path, err := compilePath(text)
	if err != nil {
		return nil, err
	}
	return &pathNode{path: path, existsCheck: existsCheck, shouldExist: shouldExist}, nil
}

// String renders the operand as JsonPath prints it in a filter.
func (n *pathNode) String() string {
	if n.existsCheck && !n.shouldExist {
		return "!" + n.path.String()
	}
	return n.path.String()
}

func (n *pathNode) equals(o valueNode) bool {
	return valueNode(n) == o
}

func (n *pathNode) asExistsCheck(shouldExist bool) *pathNode {
	return &pathNode{path: n.path, existsCheck: true, shouldExist: shouldExist}
}

// evaluate turns the path into the operand it denotes for the item under test.
func (n *pathNode) evaluate(ctx *PredicateContext) (valueNode, error) {
	if n.existsCheck {
		conf := NewConfiguration(RequireProperties)
		ec, err := n.path.evaluate(ctx.item, ctx.root, conf, false)
		if err == nil {
			_, err = ec.value()
		}
		switch {
		case err == nil:
			return trueValue, nil
		case isKind(err, ErrPathNotFound):
			return falseValue, nil
		}
		return nil, err
	}
	res, err := ctx.evaluate(n.path)
	if err != nil {
		if isKind(err, ErrPathNotFound) {
			return undefinedOperand, nil
		}
		return nil, err
	}
	switch t := res.(type) {
	case string:
		return &stringNode{value: t, singleQuote: true}, nil
	case bool:
		return boolNode(t), nil
	case nil:
		return nullValue, nil
	case *smartjson.Array, *smartjson.Object:
		return &jsonNode{value: t, parsed: true}, nil
	}
	if smartjson.IsNumber(res) {
		return newNumberNode(smartjson.ToString(res))
	}
	return nil, newError(KindJSONPath, "Could not convert class %s:%s to a ValueNode", smartjson.ClassName(res),
		smartjson.ToString(res))
}

// toValueNode converts an element of a JSON array operand.
func toValueNode(o any) (valueNode, error) {
	switch t := o.(type) {
	case nil:
		return nullValue, nil
	case string:
		if looksLikePath(t) {
			return newPathNode(t, false, false)
		}
		if looksLikeJSON(t) {
			return &jsonNode{text: t}, nil
		}
		return newStringNode(t, true)
	case bool:
		return boolNode(t), nil
	}
	if smartjson.IsNumber(o) {
		return newNumberNode(smartjson.ToString(o))
	}
	return nil, newError(KindJSONPath, "Could not determine value type")
}

func looksLikePath(s string) bool {
	str := java.Trim(s)
	if str == "" || str[0] != '@' && str[0] != '$' {
		return false
	}
	_, err := compilePath(str)
	return err == nil
}

func looksLikeJSON(s string) bool {
	str := java.Trim(s)
	if java.Len(str) <= 1 {
		return false
	}
	first, last := str[0], str[len(str)-1]
	if first == '[' && last == ']' || first == '{' && last == '}' {
		_, err := smartjson.Parse(str)
		return err == nil
	}
	return false
}

// javaEquals is Java's equals between JSON values: numbers equal only within the same type.
func javaEquals(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case *smartjson.Array:
		y, ok := b.(*smartjson.Array)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for i, e := range x.Elems() {
			if !javaEquals(e, y.Elems()[i]) {
				return false
			}
		}
		return true
	case *smartjson.Object:
		y, ok := b.(*smartjson.Object)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.Keys() {
			yv, ok := y.Get(k)
			xv, _ := x.Get(k)
			if !ok || !javaEquals(xv, yv) {
				return false
			}
		}
		return true
	case *big.Int:
		y, ok := b.(*big.Int)
		return ok && x.Cmp(y) == 0
	case float64:
		y, ok := b.(float64)
		// Double.equals compares bit patterns: NaN equals NaN, 0.0 differs from -0.0.
		return ok && (x == y && (x != 0 || isNegZero(x) == isNegZero(y)) || x != x && y != y)
	case float32:
		y, ok := b.(float32)
		return ok && (x == y && (x != 0 || isNegZero(float64(x)) == isNegZero(float64(y))) || x != x && y != y)
	case java.Decimal:
		y, ok := b.(java.Decimal)
		return ok && x.Cmp(y) == 0 && x.String() == y.String()
	}
	return a == b
}

func isNegZero(f float64) bool {
	return f == 0 && 1/f < 0
}

// escapeString escapes a string literal for printing, as JsonPath's Utils.escape with single quotes escaped.
func escapeString(s string) string {
	var b strings.Builder
	for _, ch := range java.ToChars(s) {
		switch {
		case ch > 0xfff:
			b.WriteString(`\u` + java.HexUpper(ch))
		case ch > 0xff:
			b.WriteString(`\u0` + java.HexUpper(ch))
		case ch > 0x7f:
			b.WriteString(`\u00` + java.HexUpper(ch))
		case ch < 32:
			switch ch {
			case '\b':
				b.WriteString(`\b`)
			case '\n':
				b.WriteString(`\n`)
			case '\t':
				b.WriteString(`\t`)
			case '\f':
				b.WriteString(`\f`)
			case '\r':
				b.WriteString(`\r`)
			default:
				if ch > 0xf {
					b.WriteString(`\u00` + java.HexUpper(ch))
				} else {
					b.WriteString(`\u000` + java.HexUpper(ch))
				}
			}
		case ch == '\'':
			b.WriteString(`\'`)
		case ch == '"':
			b.WriteString(`\"`)
		case ch == '\\':
			b.WriteString(`\\`)
		case ch == '/':
			b.WriteString(`\/`)
		default:
			b.WriteByte(byte(ch))
		}
	}
	return b.String()
}
