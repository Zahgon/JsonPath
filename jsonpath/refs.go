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
	"strconv"
	"strings"
)

// pathRef is the location of a result inside the document, through which the write operations change it.
type pathRef interface {
	accessor() string
	set(value any) error
	delete() error
	add(value any) error
	put(key string, value any) error
	renameKey(oldKey, newKey string) error
}

// noOpRef stands for a result that is not updated (results of a plain read).
type noOpRef struct{}

var noOp pathRef = noOpRef{}

func (noOpRef) accessor() string               { return "null" }
func (noOpRef) set(any) error                  { return nil }
func (noOpRef) delete() error                  { return nil }
func (noOpRef) add(any) error                  { return nil }
func (noOpRef) put(string, any) error          { return nil }
func (noOpRef) renameKey(string, string) error { return nil }

func targetInvalid(target any) bool {
	return target == undefined || target == nil
}

func renameInMap(target any, oldKey, newKey string) error {
	if !isMap(target) {
		return newError(KindInvalidModification, "Can only rename properties in a map")
	}
	value := getMapValue(target, oldKey)
	if value == undefined {
		return pathNotFound("No results for Key %s found in map!", oldKey)
	}
	if err := setProperty(target, newKey, value); err != nil {
		return err
	}
	return removeProperty(target, oldKey)
}

// rootRef is the document itself.
type rootRef struct {
	parent any
}

func (r *rootRef) accessor() string { return "$" }

func (r *rootRef) set(any) error {
	return newError(KindInvalidModification, "Invalid set operation")
}

func (r *rootRef) delete() error {
	return newError(KindInvalidModification, "Invalid delete operation")
}

func (r *rootRef) add(value any) error {
	if !isArray(r.parent) {
		return newError(KindInvalidModification, "Invalid add operation. $ is not an array")
	}
	n, err := length(r.parent)
	if err != nil {
		return err
	}
	return setArrayIndex(r.parent, n, value)
}

func (r *rootRef) put(key string, value any) error {
	if !isMap(r.parent) {
		return newError(KindInvalidModification, "Invalid put operation. $ is not a map")
	}
	return setProperty(r.parent, key, value)
}

func (r *rootRef) renameKey(oldKey, newKey string) error {
	if targetInvalid(r.parent) {
		return nil
	}
	return renameInMap(r.parent, oldKey, newKey)
}

// arrayIndexRef is an element of an array, by the index the path named (possibly negative).
type arrayIndexRef struct {
	parent any
	index  int
}

func (r *arrayIndexRef) accessor() string { return strconv.Itoa(r.index) }

func (r *arrayIndexRef) set(value any) error {
	return setArrayIndex(r.parent, r.index, value)
}

func (r *arrayIndexRef) delete() error {
	return removeProperty(r.parent, r.index)
}

func (r *arrayIndexRef) add(value any) error {
	target, err := getArrayIndex(r.parent, r.index)
	if err != nil || targetInvalid(target) {
		return err
	}
	if !isArray(target) {
		return newError(KindInvalidModification, "Can only add to an array")
	}
	// JsonPath adds through the map-only setProperty here, so adding to a nested array always fails.
	return setProperty(target, "null", value)
}

func (r *arrayIndexRef) put(key string, value any) error {
	target, err := getArrayIndex(r.parent, r.index)
	if err != nil || targetInvalid(target) {
		return err
	}
	if !isMap(target) {
		return newError(KindInvalidModification, "Can only add properties to a map")
	}
	return setProperty(target, key, value)
}

func (r *arrayIndexRef) renameKey(oldKey, newKey string) error {
	target, err := getArrayIndex(r.parent, r.index)
	if err != nil || targetInvalid(target) {
		return err
	}
	return renameInMap(target, oldKey, newKey)
}

// objectPropertyRef is a property of an object.
type objectPropertyRef struct {
	parent   any
	property string
}

func (r *objectPropertyRef) accessor() string { return r.property }

func (r *objectPropertyRef) set(value any) error {
	return setProperty(r.parent, r.property, value)
}

func (r *objectPropertyRef) delete() error {
	return removeProperty(r.parent, r.property)
}

func (r *objectPropertyRef) add(value any) error {
	target := getMapValue(r.parent, r.property)
	if targetInvalid(target) {
		return nil
	}
	if !isArray(target) {
		return newError(KindInvalidModification, "Can only add to an array")
	}
	n, err := length(target)
	if err != nil {
		return err
	}
	return setArrayIndex(target, n, value)
}

func (r *objectPropertyRef) put(key string, value any) error {
	target := getMapValue(r.parent, r.property)
	if targetInvalid(target) {
		return nil
	}
	if !isMap(target) {
		return newError(KindInvalidModification, "Can only add properties to a map")
	}
	return setProperty(target, key, value)
}

func (r *objectPropertyRef) renameKey(oldKey, newKey string) error {
	target := getMapValue(r.parent, r.property)
	if targetInvalid(target) {
		return nil
	}
	return renameInMap(target, oldKey, newKey)
}

// multiPropertyRef is the set of properties a ['a','b'] leaf merged.
type multiPropertyRef struct {
	parent     any
	properties []string
}

func (r *multiPropertyRef) accessor() string { return strings.Join(r.properties, "&&") }

func (r *multiPropertyRef) set(value any) error {
	for _, p := range r.properties {
		if err := setProperty(r.parent, p, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *multiPropertyRef) delete() error {
	for _, p := range r.properties {
		if err := removeProperty(r.parent, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *multiPropertyRef) add(any) error {
	return newError(KindInvalidModification, "Add can not be performed to multiple properties")
}

func (r *multiPropertyRef) put(string, any) error {
	return newError(KindInvalidModification, "Put can not be performed to multiple properties")
}

func (r *multiPropertyRef) renameKey(string, string) error {
	return newError(KindInvalidModification, "Rename can not be performed to multiple properties")
}
