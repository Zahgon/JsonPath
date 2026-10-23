// Copyright 2011-2023 JSON-SMART authors: json-smart 2.5.0, the JSON provider of Jayway JsonPath, ported to Go.
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

// Package smartjson is the JSON model JsonPath works on: the values json-smart's permissive parser produces, the
// parser itself and json-smart's serialisation.
//
// A document value is one of:
//
//	nil                    JSON null
//	bool                   true or false
//	string                 a string
//	int32, int64, *big.Int an integer (Integer, Long or BigInteger, by magnitude)
//	float64, float32       a binary floating-point number (Double, or Float for NaN)
//	BigDecimal             a decimal number too long for a Double
//	*Object                an object keeping its keys in insertion order
//	*Array                 an array
//
// KeySet is the read-only key view the keys() function returns.
package smartjson

import (
	"math/big"

	"github.com/json-path/JsonPath/internal/java"
)

// BigDecimal is an arbitrary-precision decimal number with Java BigDecimal semantics.
type BigDecimal = java.Decimal

// Object is a JSON object whose keys keep their insertion order; putting an existing key keeps its position.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{values: map[string]any{}}
}

// Len returns the number of keys.
func (o *Object) Len() int {
	return len(o.keys)
}

// Keys returns the keys in order. The slice must not be modified.
func (o *Object) Keys() []string {
	return o.keys
}

// Get returns the value of key and whether the key is present.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.values[key]
	return v, ok
}

// Put sets key to value, appending the key if it is new.
func (o *Object) Put(key string, value any) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// Remove deletes key if present.
func (o *Object) Remove(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
}

// Array is a JSON array.
type Array struct {
	elems []any
}

// NewArray returns an array holding elems.
func NewArray(elems ...any) *Array {
	return &Array{elems: elems}
}

// Len returns the number of elements.
func (a *Array) Len() int {
	return len(a.elems)
}

// Elems returns the elements. The slice must not be modified.
func (a *Array) Elems() []any {
	return a.elems
}

// Get returns element i; ok is false when i is out of range.
func (a *Array) Get(i int) (value any, ok bool) {
	if i < 0 || i >= len(a.elems) {
		return nil, false
	}
	return a.elems[i], true
}

// Set replaces element i, or appends when i equals the length; ok is false when i is out of range.
func (a *Array) Set(i int, value any) (ok bool) {
	switch {
	case i == len(a.elems):
		a.elems = append(a.elems, value)
	case i < 0 || i > len(a.elems):
		return false
	default:
		a.elems[i] = value
	}
	return true
}

// Add appends value.
func (a *Array) Add(value any) {
	a.elems = append(a.elems, value)
}

// Remove deletes element i; ok is false when i is out of range.
func (a *Array) Remove(i int) (ok bool) {
	if i < 0 || i >= len(a.elems) {
		return false
	}
	a.elems = append(a.elems[:i:i], a.elems[i+1:]...)
	return true
}

// KeySet is the live key view of an object, as returned by the keys() function.
type KeySet struct {
	Object *Object
}

// IsNumber reports whether v is one of the number types.
func IsNumber(v any) bool {
	switch v.(type) {
	case int32, int64, *big.Int, float64, float32, BigDecimal:
		return true
	}
	return false
}

// ClassName returns the Java class name of the value json-smart would hold for v.
func ClassName(v any) string {
	switch v.(type) {
	case *Object:
		return "java.util.LinkedHashMap"
	case *Array:
		return "net.minidev.json.JSONArray"
	case KeySet:
		return "java.util.LinkedHashMap$LinkedKeySet"
	case string:
		return "java.lang.String"
	case bool:
		return "java.lang.Boolean"
	case int32:
		return "java.lang.Integer"
	case int64:
		return "java.lang.Long"
	case *big.Int:
		return "java.math.BigInteger"
	case float64:
		return "java.lang.Double"
	case float32:
		return "java.lang.Float"
	case BigDecimal:
		return "java.math.BigDecimal"
	}
	return "java.lang.Object"
}
