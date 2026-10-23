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

// Package jsonpath reads and updates JSON documents with Jayway JsonPath expressions such as
// $.store.book[?(@.price < 10)].title, with Jayway JsonPath's semantics: its path syntax and normalised paths,
// filters and their operators, path functions, evaluation options and error messages, on json-smart's JSON model
// (see package smartjson).
//
// Documents are the values smartjson.Parse returns. Write operations change the document in place.
package jsonpath

import "github.com/json-path/JsonPath/smartjson"

// JSONPath is a compiled path expression.
type JSONPath struct {
	path *compiledPath
}

// Compile compiles a path expression; its [?] placeholders take the filters in order.
func Compile(path string, filters ...Predicate) (*JSONPath, error) {
	if path == "" {
		return nil, newError(KindIllegalArgument, "json can not be null or empty")
	}
	p, err := compilePath(path, filters...)
	if err != nil {
		return nil, err
	}
	return &JSONPath{path: p}, nil
}

// IsPathDefinite reports whether a path selects at most one value.
func IsPathDefinite(path string) (bool, error) {
	p, err := Compile(path)
	if err != nil {
		return false, err
	}
	return p.IsDefinite(), nil
}

// String returns the path in normalised bracket notation, as $['store']['book'][*].
func (p *JSONPath) String() string {
	return p.path.String()
}

// IsDefinite reports whether the path selects at most one value: it has no deep scan, wildcard, filter, slice or
// multiple index.
func (p *JSONPath) IsDefinite() bool {
	return p.path.isDefinite()
}

// Read evaluates the path against a document. A definite path returns the value it selects, any other path an
// array of values; the options of conf change this (see Option).
func (p *JSONPath) Read(document any, conf *Configuration) (any, error) {
	path := p.path
	asPathList := conf.Has(AsPathList)
	alwaysList := conf.Has(AlwaysReturnList)
	suppress := conf.Has(SuppressExceptions)
	empty := func() any {
		if path.isDefinite() {
			return nil
		}
		return smartjson.NewArray()
	}
	if path.isFunctionPath() {
		if asPathList || alwaysList {
			if suppress {
				return empty(), nil
			}
			return nil, newError(KindJSONPath, "Options AS_PATH_LIST and ALWAYS_RETURN_LIST are not allowed when using path functions!")
		}
		ec, err := path.evaluate(document, document, conf, false)
		if err != nil {
			return nil, err
		}
		if suppress && ec.resultIndex == 0 {
			return empty(), nil
		}
		return ec.value()
	}
	ec, err := path.evaluate(document, document, conf, false)
	if err != nil {
		return nil, err
	}
	if asPathList {
		if suppress && ec.resultIndex == 0 {
			return smartjson.NewArray(), nil
		}
		return ec.paths()
	}
	if suppress && ec.resultIndex == 0 {
		if alwaysList {
			return smartjson.NewArray(), nil
		}
		return empty(), nil
	}
	res, err := ec.value()
	if err != nil {
		return nil, err
	}
	if alwaysList && path.isDefinite() {
		return smartjson.NewArray(res), nil
	}
	return res, nil
}

// update evaluates the path for an update and applies op to every location found, in JsonPath's order.
func (p *JSONPath) update(document any, conf *Configuration, failIfMissing bool, op func(pathRef) error) (any, error) {
	if document == nil {
		return nil, newError(KindIllegalArgument, "json can not be null")
	}
	ec, err := p.path.evaluate(document, document, conf, true)
	if err != nil {
		return nil, err
	}
	if failIfMissing && ec.resultIndex == 0 {
		if conf.Has(SuppressExceptions) {
			return p.missingResult(conf), nil
		}
		return nil, &Error{Kind: KindPathNotFound}
	}
	for _, ref := range ec.updateOperations() {
		if err := op(ref); err != nil {
			return nil, err
		}
	}
	if conf.Has(AsPathList) {
		paths := smartjson.NewArray()
		for _, s := range ec.pathList() {
			paths.Add(s)
		}
		return paths, nil
	}
	return document, nil
}

func (p *JSONPath) missingResult(conf *Configuration) any {
	if conf.Has(AsPathList) || conf.Has(AlwaysReturnList) || !p.path.isDefinite() {
		return smartjson.NewArray()
	}
	return nil
}

// Set replaces every value the path selects. It returns the document, or with AsPathList the paths changed.
func (p *JSONPath) Set(document, value any, conf *Configuration) (any, error) {
	return p.update(document, conf, true, func(r pathRef) error { return r.set(value) })
}

// Delete removes every value the path selects.
func (p *JSONPath) Delete(document any, conf *Configuration) (any, error) {
	return p.update(document, conf, true, func(r pathRef) error { return r.delete() })
}

// Add appends value to every array the path selects.
func (p *JSONPath) Add(document, value any, conf *Configuration) (any, error) {
	return p.update(document, conf, true, func(r pathRef) error { return r.add(value) })
}

// Put sets key to value in every object the path selects.
func (p *JSONPath) Put(document any, key string, value any, conf *Configuration) (any, error) {
	if key == "" {
		return nil, newError(KindIllegalArgument, "key can not be null or empty")
	}
	return p.update(document, conf, true, func(r pathRef) error { return r.put(key, value) })
}

// RenameKey renames a key of every object the path selects. With SuppressExceptions a failure at one location
// does not stop the others.
func (p *JSONPath) RenameKey(document any, oldKey, newKey string, conf *Configuration) (any, error) {
	if newKey == "" {
		return nil, newError(KindIllegalArgument, "newKeyName can not be null or empty")
	}
	return p.update(document, conf, false, func(r pathRef) error {
		if err := r.renameKey(oldKey, newKey); err != nil && !conf.Has(SuppressExceptions) {
			return err
		}
		return nil
	})
}

// Parse parses JSON text with json-smart's permissive parser.
func Parse(text string) (any, error) {
	return parseJSON(text)
}

// DocumentContext is a parsed document with the configuration to evaluate paths against it.
type DocumentContext struct {
	json any
	conf *Configuration
}

// NewDocumentContext wraps a parsed document.
func NewDocumentContext(document any, conf *Configuration) (*DocumentContext, error) {
	if document == nil {
		return nil, newError(KindIllegalArgument, "json object can not be null")
	}
	return &DocumentContext{json: document, conf: conf}, nil
}

// ParseDocument parses JSON text into a document context.
func ParseDocument(text string, conf *Configuration) (*DocumentContext, error) {
	if text == "" {
		return nil, newError(KindIllegalArgument, "json string can not be null or empty")
	}
	doc, err := parseJSON(text)
	if err != nil {
		return nil, err
	}
	return &DocumentContext{json: doc, conf: conf}, nil
}

// JSON returns the document.
func (c *DocumentContext) JSON() any {
	return c.json
}

// JSONString serialises the document as json-smart does.
func (c *DocumentContext) JSONString() string {
	return smartjson.Marshal(c.json, smartjson.StyleCompact)
}

// Configuration returns the context's configuration.
func (c *DocumentContext) Configuration() *Configuration {
	return c.conf
}

// Limit returns a context whose reads stop after max results.
func (c *DocumentContext) Limit(max int) *DocumentContext {
	return &DocumentContext{json: c.json, conf: c.conf.WithListeners(func(found FoundResult) EvaluationContinuation {
		if found.Index == max-1 {
			return Abort
		}
		return Continue
	})}
}

func (c *DocumentContext) compile(path string, filters []Predicate) (*JSONPath, error) {
	if path == "" {
		return nil, newError(KindIllegalArgument, "path can not be null or empty")
	}
	return Compile(path, filters...)
}

// Read evaluates a path against the document.
func (c *DocumentContext) Read(path string, filters ...Predicate) (any, error) {
	p, err := c.compile(path, filters)
	if err != nil {
		return nil, err
	}
	return p.Read(c.json, c.conf)
}

// Set replaces the values the path selects and returns the context.
func (c *DocumentContext) Set(path string, value any, filters ...Predicate) (*DocumentContext, error) {
	return c.write(path, filters, func(p *JSONPath, conf *Configuration) error {
		_, err := p.Set(c.json, value, conf)
		return err
	})
}

// Delete removes the values the path selects and returns the context.
func (c *DocumentContext) Delete(path string, filters ...Predicate) (*DocumentContext, error) {
	return c.write(path, filters, func(p *JSONPath, conf *Configuration) error {
		_, err := p.Delete(c.json, conf)
		return err
	})
}

// Add appends value to the arrays the path selects and returns the context.
func (c *DocumentContext) Add(path string, value any, filters ...Predicate) (*DocumentContext, error) {
	return c.write(path, filters, func(p *JSONPath, conf *Configuration) error {
		_, err := p.Add(c.json, value, conf)
		return err
	})
}

// Put sets key in the objects the path selects and returns the context.
func (c *DocumentContext) Put(path, key string, value any, filters ...Predicate) (*DocumentContext, error) {
	return c.write(path, filters, func(p *JSONPath, conf *Configuration) error {
		_, err := p.Put(c.json, key, value, conf)
		return err
	})
}

// RenameKey renames a key in the objects the path selects and returns the context.
func (c *DocumentContext) RenameKey(path, oldKey, newKey string, filters ...Predicate) (*DocumentContext, error) {
	return c.write(path, filters, func(p *JSONPath, conf *Configuration) error {
		_, err := p.RenameKey(c.json, oldKey, newKey, conf)
		return err
	})
}

func (c *DocumentContext) write(path string, filters []Predicate, op func(*JSONPath, *Configuration) error) (*DocumentContext, error) {
	p, err := Compile(path, filters...)
	if err != nil {
		return nil, err
	}
	if err := op(p, c.conf.WithOptions(AsPathList)); err != nil {
		return nil, err
	}
	return c, nil
}
