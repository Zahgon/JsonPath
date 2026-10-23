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
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/smartjson"
)

type paramKind int

const (
	paramNone paramKind = iota
	paramJSON
	paramPath
)

// parameter is an argument of a path function: JSON text or a path, bound to its value when the function runs.
type parameter struct {
	kind      paramKind
	json      string
	path      *compiledPath
	binding   binding
	evaluated bool
}

func (p *parameter) value() (any, error) {
	return p.binding.get()
}

type binding interface {
	get() (any, error)
}

// jsonBinding parses its JSON text every time the value is used.
type jsonBinding struct {
	json string
}

func (b *jsonBinding) get() (any, error) {
	return parseJSON(b.json)
}

// pathBinding holds the value of a path argument, evaluated against the document when the function runs.
type pathBinding struct {
	path     *compiledPath
	rootText string
	conf     *Configuration
	result   any
}

func newPathBinding(path *compiledPath, root any, conf *Configuration) (*pathBinding, error) {
	ec, err := path.evaluate(root, root, conf, false)
	if err != nil {
		return nil, err
	}
	result, err := ec.value()
	if err != nil {
		return nil, err
	}
	return &pathBinding{path: path, rootText: smartjson.ToString(root), conf: conf, result: result}, nil
}

func (b *pathBinding) get() (any, error) {
	return b.result, nil
}

// equals reports whether two bindings are for the same path, document text and configuration.
func (b *pathBinding) equals(other binding) bool {
	o, ok := other.(*pathBinding)
	return ok && o.path == b.path && o.rootText == b.rootText && o.conf == b.conf
}

// pathFunction computes a function's result from the model it is applied to and its parameters.
type pathFunction func(currentPath string, parent pathRef, model any, ctx *evalContext, params []*parameter) (any, error)

func newPathFunction(name string) (pathFunction, error) {
	switch name {
	case "avg":
		return aggregate(&average{}), nil
	case "stddev":
		return aggregate(&stddev{}), nil
	case "sum":
		return aggregate(&sum{}), nil
	case "min":
		return aggregate(&minimum{value: math.MaxFloat64}), nil
	case "max":
		return aggregate(&maximum{value: math.SmallestNonzeroFloat64}), nil
	case "concat":
		return concatenate, nil
	case "length", "size":
		return lengthOf, nil
	case "append":
		return appendValues, nil
	case "keys":
		return keySet, nil
	case "first":
		return sequenceAt(func([]*parameter, *evalContext) (int, error) { return 0, nil }), nil
	case "last":
		return sequenceAt(func([]*parameter, *evalContext) (int, error) { return -1, nil }), nil
	case "index":
		return sequenceAt(indexFromParameters), nil
	}
	return nil, invalidPath("Function with name: %s does not exist.", name)
}

// parameterValues flattens the parameter values (array values contribute their elements) and keeps those keep
// accepts, converted.
func parameterValues(params []*parameter, keep func(any) (any, bool)) ([]any, error) {
	var out []any
	for _, p := range params {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		items := []any{v}
		if arr, ok := v.(*smartjson.Array); ok {
			items = arr.Elems()
		}
		for _, item := range items {
			if c, ok := keep(item); ok {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func keepNumbers(v any) (any, bool) {
	return v, smartjson.IsNumber(v)
}

func keepStrings(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	return smartjson.ToString(v), true
}

// doubleValue converts a number as Java's Number.doubleValue.
func doubleValue(v any) float64 {
	switch t := v.(type) {
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case *big.Int:
		return java.BigIntToFloat64(t)
	case float64:
		return t
	case float32:
		return float64(t)
	case java.Decimal:
		return t.Float64()
	}
	return 0
}

// intValue converts a number as Java's Number.intValue.
func intValue(v any) int32 {
	switch t := v.(type) {
	case int32:
		return t
	case int64:
		return int32(t)
	case *big.Int:
		return java.BigIntToInt32(t)
	case float64:
		return java.IntValueOfDouble(t)
	case float32:
		return java.IntValueOfDouble(float64(t))
	case java.Decimal:
		return t.IntValue()
	}
	return 0
}

type accumulator interface {
	next(v float64)
	result() float64
}

type average struct{ sum, count float64 }

func (a *average) next(v float64) { a.count++; a.sum += v }
func (a *average) result() float64 {
	if a.count != 0 {
		return a.sum / a.count
	}
	return 0
}

type stddev struct{ sum, sumSq, count float64 }

func (s *stddev) next(v float64) {
	s.sum += v
	// The explicit conversion rounds the product before the addition, as Java does, so it is never fused.
	s.sumSq += float64(v * v)
	s.count++
}
func (s *stddev) result() float64 {
	return math.Sqrt(s.sumSq/s.count - s.sum*s.sum/s.count/s.count)
}

type sum struct{ total float64 }

func (s *sum) next(v float64)  { s.total += v }
func (s *sum) result() float64 { return s.total }

type minimum struct{ value float64 }

func (m *minimum) next(v float64) {
	if m.value > v {
		m.value = v
	}
}
func (m *minimum) result() float64 { return m.value }

// maximum starts from the smallest positive double, as JsonPath's max() does, so it never returns a negative.
type maximum struct{ value float64 }

func (m *maximum) next(v float64) {
	if m.value < v {
		m.value = v
	}
}
func (m *maximum) result() float64 { return m.value }

// aggregate applies a numeric aggregation to the numbers of an array model and of the parameters.
func aggregate(acc accumulator) pathFunction {
	return func(_ string, _ pathRef, model any, _ *evalContext, params []*parameter) (any, error) {
		count := 0
		if arr, ok := model.(*smartjson.Array); ok {
			for _, v := range arr.Elems() {
				if smartjson.IsNumber(v) {
					count++
					acc.next(doubleValue(v))
				}
			}
		}
		values, err := parameterValues(params, keepNumbers)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			count++
			acc.next(doubleValue(v))
		}
		if count == 0 {
			return nil, newError(KindJSONPath, "Aggregation function attempted to calculate value using empty array")
		}
		return acc.result(), nil
	}
}

func concatenate(_ string, _ pathRef, model any, _ *evalContext, params []*parameter) (any, error) {
	var b strings.Builder
	if arr, ok := model.(*smartjson.Array); ok {
		for _, v := range arr.Elems() {
			if s, ok := v.(string); ok {
				b.WriteString(s)
			}
		}
	}
	values, err := parameterValues(params, keepStrings)
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		b.WriteString(v.(string))
	}
	return b.String(), nil
}

// lengthOf is length() and size(): the size of an array or object. With a path argument it appends a wildcard to
// that path (for good, as JsonPath does) and counts what the path selects from the model.
func lengthOf(_ string, _ pathRef, model any, ctx *evalContext, params []*parameter) (any, error) {
	if len(params) > 0 {
		path := params[0].path
		if path == nil {
			return nil, &Error{Kind: KindNullPointer}
		}
		if !path.isFunctionPath() {
			tail := path.root.next
			for tail != nil && tail.base().next != nil {
				tail = tail.base().next
			}
			if tail != nil {
				tail.base().next = newWildcardToken()
			}
		}
		ec, err := path.evaluate(model, model, ctx.conf, false)
		if err != nil {
			return nil, err
		}
		inner, err := ec.value()
		if err != nil {
			return nil, err
		}
		if arr, ok := inner.(*smartjson.Array); ok {
			return int32(arr.Len()), nil
		}
	}
	switch t := model.(type) {
	case *smartjson.Array:
		return int32(t.Len()), nil
	case *smartjson.Object:
		return int32(t.Len()), nil
	}
	return nil, nil
}

func appendValues(_ string, _ pathRef, model any, _ *evalContext, params []*parameter) (any, error) {
	for _, p := range params {
		arr, ok := model.(*smartjson.Array)
		if !ok {
			continue
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		arr.Add(v)
	}
	return model, nil
}

func keySet(_ string, _ pathRef, model any, _ *evalContext, _ []*parameter) (any, error) {
	if obj, ok := model.(*smartjson.Object); ok {
		return smartjson.KeySet{Object: obj}, nil
	}
	return nil, nil
}

func indexFromParameters(params []*parameter, _ *evalContext) (int, error) {
	values, err := parameterValues(params, keepNumbers)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, indexOutOfBounds(0, 0)
	}
	return int(intValue(values[0])), nil
}

// sequenceAt picks one element of an array model: first(), last() or index(n), counting negative positions from
// the end.
func sequenceAt(target func([]*parameter, *evalContext) (int, error)) pathFunction {
	return func(_ string, _ pathRef, model any, ctx *evalContext, params []*parameter) (any, error) {
		arr, ok := model.(*smartjson.Array)
		if !ok {
			return nil, newError(KindJSONPath, "Aggregation function attempted to calculate value using empty array")
		}
		items := arr.Elems()
		idx, err := target(params, ctx)
		if err != nil {
			return nil, err
		}
		if idx >= 0 {
			if idx >= len(items) {
				return nil, indexOutOfBounds(idx, len(items))
			}
			return items[idx], nil
		}
		real := len(items) + idx
		if real > 0 {
			return items[real], nil
		}
		return nil, newError(KindJSONPath, "Target index:%s larger than object count:%d", strconv.Itoa(idx), len(items))
	}
}
