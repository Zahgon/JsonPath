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
	"reflect"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
)

type evaluator func(left, right valueNode) (bool, error)

var evaluators map[relationalOperator]evaluator

func init() {
	evaluators = map[relationalOperator]evaluator{
		opExists:   evalExists,
		opNE:       negate(evalEquals),
		opTSNE:     negate(evalTypeSafeEquals),
		opEQ:       evalEquals,
		opTSEQ:     evalTypeSafeEquals,
		opLT:       compareWith(func(c int) bool { return c < 0 }),
		opLTE:      compareWith(func(c int) bool { return c <= 0 }),
		opGT:       compareWith(func(c int) bool { return c > 0 }),
		opGTE:      compareWith(func(c int) bool { return c >= 0 }),
		opRegex:    evalRegex,
		opSize:     evalSize,
		opEmpty:    evalEmpty,
		opIN:       evalIn,
		opNIN:      negate(evalIn),
		opAll:      evalAll,
		opContains: evalContains,
		opMatches:  operandOnly("predicate"),
		opType:     operandOnly("class"),
		opSubsetOf: evalSubsetOf,
		opAnyOf:    evalAnyOf,
		opNoneOf:   evalNoneOf,
	}
}

func negate(e evaluator) evaluator {
	return func(l, r valueNode) (bool, error) {
		ok, err := e(l, r)
		return !ok && err == nil, err
	}
}

func expected(what string) error {
	return invalidPath("Expected %s node", what)
}

func asBoolean(n valueNode) (*booleanNode, error) {
	if b, ok := n.(*booleanNode); ok {
		return b, nil
	}
	return nil, expected("boolean")
}

func asJSON(n valueNode) (*jsonNode, error) {
	if j, ok := n.(*jsonNode); ok {
		return j, nil
	}
	return nil, expected("json")
}

func asValueList(n valueNode) (*valueListNode, error) {
	if l, ok := n.(*valueListNode); ok {
		return l, nil
	}
	return nil, expected("value list")
}

func evalExists(l, r valueNode) (bool, error) {
	lb, lok := l.(*booleanNode)
	rb, rok := r.(*booleanNode)
	if !lok && !rok {
		return false, newError(KindJSONPath, "Failed to evaluate exists expression")
	}
	if !lok {
		return false, expected("boolean")
	}
	if !rok {
		return false, expected("boolean")
	}
	return lb.value == rb.value, nil
}

func evalEquals(l, r valueNode) (bool, error) {
	lj, lok := l.(*jsonNode)
	rj, rok := r.(*jsonNode)
	if lok && rok {
		return lj.equalsJSON(rj)
	}
	return l.equals(r), nil
}

func evalTypeSafeEquals(l, r valueNode) (bool, error) {
	if reflect.TypeOf(l) != reflect.TypeOf(r) {
		return false, nil
	}
	return evalEquals(l, r)
}

// compareWith builds the ordering operators: numbers compare by value, strings by UTF-16 code units, anything
// else is false.
func compareWith(accept func(int) bool) evaluator {
	return func(l, r valueNode) (bool, error) {
		ln, lnum := l.(*numberNode)
		rn, rnum := r.(*numberNode)
		if lnum && rnum {
			return accept(ln.number.Cmp(*rn.number)), nil
		}
		ls, lstr := l.(*stringNode)
		rs, rstr := r.(*stringNode)
		if lstr && rstr {
			return accept(java.Compare(ls.value, rs.value)), nil
		}
		return false, nil
	}
}

func evalSize(l, r valueNode) (bool, error) {
	rn, ok := r.(*numberNode)
	if !ok {
		return false, nil
	}
	size := int(rn.number.IntValue())
	switch t := l.(type) {
	case *stringNode:
		return java.Len(t.value) == size, nil
	case *jsonNode:
		n, err := t.length()
		return n == size, err
	}
	return false, nil
}

func evalEmpty(l, r valueNode) (bool, error) {
	switch t := l.(type) {
	case *stringNode:
		b, err := asBoolean(r)
		if err != nil {
			return false, err
		}
		return (t.value == "") == b.value, nil
	case *jsonNode:
		empty, err := t.isEmpty()
		if err != nil {
			return false, err
		}
		b, err := asBoolean(r)
		if err != nil {
			return false, err
		}
		return empty == b.value, nil
	}
	return false, nil
}

// listOperand converts an operand into a value list: JSON arrays convert, other JSON is undefined (nil).
func listOperand(n valueNode) (*valueListNode, error) {
	if j, ok := n.(*jsonNode); ok {
		v, err := j.asValueListNode()
		if err != nil {
			return nil, err
		}
		if list, ok := v.(*valueListNode); ok {
			return list, nil
		}
		return nil, nil
	}
	return asValueList(n)
}

func evalIn(l, r valueNode) (bool, error) {
	list, err := listOperand(r)
	if list == nil || err != nil {
		return false, err
	}
	return list.contains(l), nil
}

func evalAll(l, r valueNode) (bool, error) {
	required, err := asValueList(r)
	if err != nil {
		return false, err
	}
	j, ok := l.(*jsonNode)
	if !ok {
		return false, nil
	}
	v, err := j.asValueListNode()
	if err != nil {
		return false, err
	}
	if list, ok := v.(*valueListNode); ok {
		for _, req := range required.nodes {
			if !list.contains(req) {
				return false, nil
			}
		}
	}
	return true, nil
}

func evalContains(l, r valueNode) (bool, error) {
	ls, lstr := l.(*stringNode)
	rs, rstr := r.(*stringNode)
	if lstr && rstr {
		return strings.Contains(ls.value, rs.value), nil
	}
	j, ok := l.(*jsonNode)
	if !ok {
		return false, nil
	}
	v, err := j.asValueListNode()
	if err != nil {
		return false, err
	}
	list, ok := v.(*valueListNode)
	if !ok {
		return false, nil
	}
	return list.contains(r), nil
}

// operandOnly builds the evaluators of MATCHES and TYPE. They take a predicate or a class operand, which only
// JsonPath's programmatic Criteria builder can supply; a filter written as text never produces one. As in the
// original, applying them therefore fails with the expected-operand error, which rejects the item under test (and
// so propagates through a negation instead of being turned into true).
func operandOnly(kind string) evaluator {
	return func(_, _ valueNode) (bool, error) {
		return false, expected(kind)
	}
}

func evalRegex(l, r valueNode) (bool, error) {
	lp, lpat := l.(*patternNode)
	rp, rpat := r.(*patternNode)
	if lpat == rpat {
		return false, nil
	}
	pattern, subject := lp, r
	if rpat {
		pattern, subject = rp, l
	}
	isList := false
	if _, ok := subject.(*valueListNode); ok {
		isList = true
	} else if j, ok := subject.(*jsonNode); ok {
		arr, err := j.isArray()
		if err != nil {
			return false, err
		}
		isList = arr
	}
	if isList {
		j, err := asJSON(subject)
		if err != nil {
			return false, err
		}
		v, err := j.asValueListNode()
		if err != nil {
			return false, err
		}
		list, ok := v.(*valueListNode)
		if !ok {
			return false, nil
		}
		for _, n := range list.nodes {
			if pattern.re.Matches(regexInput(n)) {
				return true, nil
			}
		}
		return false, nil
	}
	return pattern.re.Matches(regexInput(subject)), nil
}

// regexInput is the text a regular expression is matched against: strings and numbers as text, booleans as
// true/false, anything else as the empty string.
func regexInput(n valueNode) string {
	switch t := n.(type) {
	case *stringNode:
		return t.value
	case *numberNode:
		return t.number.String()
	case *booleanNode:
		return t.String()
	}
	return ""
}

func bothLists(l, r valueNode) (*valueListNode, *valueListNode, error) {
	right, err := listOperand(r)
	if right == nil || err != nil {
		return nil, nil, err
	}
	left, err := listOperand(l)
	if left == nil || err != nil {
		return nil, nil, err
	}
	return left, right, nil
}

func evalSubsetOf(l, r valueNode) (bool, error) {
	left, right, err := bothLists(l, r)
	if left == nil || err != nil {
		return false, err
	}
	return left.subsetOf(right), nil
}

func evalAnyOf(l, r valueNode) (bool, error) {
	left, right, err := bothLists(l, r)
	if left == nil || err != nil {
		return false, err
	}
	for _, a := range left.nodes {
		for _, b := range right.nodes {
			if a.equals(b) {
				return true, nil
			}
		}
	}
	return false, nil
}

func evalNoneOf(l, r valueNode) (bool, error) {
	left, right, err := bothLists(l, r)
	if left == nil || err != nil {
		return false, err
	}
	for _, a := range left.nodes {
		for _, b := range right.nodes {
			if a.equals(b) {
				return false, nil
			}
		}
	}
	return true, nil
}
