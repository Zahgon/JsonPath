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
	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/smartjson"
)

// The JSON provider: how the evaluation reads and changes json-smart values.

// absentValue marks a property that is absent, as opposed to one holding null.
type absentValue struct{}

var undefined any = absentValue{}

func isArray(v any) bool {
	_, ok := v.(*smartjson.Array)
	return ok
}

func isMap(v any) bool {
	_, ok := v.(*smartjson.Object)
	return ok
}

func indexOutOfBounds(index, length int) *Error {
	return newError(KindIndexOutOfBounds, "Index %d out of bounds for length %d", index, length)
}

func classCast(v any) *Error {
	return newError(KindClassCast, "class %s cannot be cast", smartjson.ClassName(v))
}

func getArrayIndex(v any, index int) (any, error) {
	arr, ok := v.(*smartjson.Array)
	if !ok {
		return nil, classCast(v)
	}
	e, ok := arr.Get(index)
	if !ok {
		return nil, indexOutOfBounds(index, arr.Len())
	}
	return e, nil
}

func setArrayIndex(v any, index int, value any) error {
	arr, ok := v.(*smartjson.Array)
	if !ok {
		return &Error{Kind: KindUnsupportedOperation}
	}
	if !arr.Set(index, value) {
		return indexOutOfBounds(index, arr.Len())
	}
	return nil
}

// getMapValue returns the value of key in an object, or undefined when the key is absent.
func getMapValue(v any, key string) any {
	val, ok := v.(*smartjson.Object).Get(key)
	if !ok {
		return undefined
	}
	return val
}

// setProperty puts key into an object; on anything else it fails with the class name as message, as JsonPath does.
func setProperty(v any, key string, value any) error {
	obj, ok := v.(*smartjson.Object)
	if !ok {
		if v == nil {
			return &Error{Kind: KindNullPointer}
		}
		return newError(KindJSONPath, "%s", smartjson.ClassName(v))
	}
	obj.Put(key, value)
	return nil
}

// removeProperty removes a key from an object or an index from an array.
func removeProperty(v any, key any) error {
	if obj, ok := v.(*smartjson.Object); ok {
		obj.Remove(smartjson.ToString(key))
		return nil
	}
	arr, ok := v.(*smartjson.Array)
	if !ok {
		return classCast(v)
	}
	index := key.(int)
	if !arr.Remove(index) {
		return indexOutOfBounds(index, arr.Len())
	}
	return nil
}

func length(v any) (int, error) {
	switch t := v.(type) {
	case *smartjson.Array:
		return t.Len(), nil
	case *smartjson.Object:
		return t.Len(), nil
	case string:
		return java.Len(t), nil
	case nil:
		return 0, newError(KindJSONPath, "length operation cannot be applied to null")
	}
	return 0, newError(KindJSONPath, "length operation cannot be applied to %s", smartjson.ClassName(v))
}

// parseJSON parses a document or value with json-smart's permissive parser.
func parseJSON(text string) (any, error) {
	v, err := smartjson.Parse(text)
	if err != nil {
		return nil, wrap(KindInvalidJSON, err)
	}
	return v, nil
}
