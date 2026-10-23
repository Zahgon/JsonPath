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

	"github.com/json-path/JsonPath/smartjson"
)

// pathToken is one step of a compiled path. Tokens form a doubly linked chain from the root token; each evaluates
// itself against a model and hands what it selects to the next token, or records it as a result when it is the leaf.
type pathToken interface {
	base() *tokenBase
	evaluate(currentPath string, parent pathRef, model any, ctx *evalContext) error
	isTokenDefinite() bool
	pathFragment() string
}

// tokenBase holds the chain links and the per-token state JsonPath keeps between evaluations.
type tokenBase struct {
	prev, next         pathToken
	definite           *bool
	upstreamDefinite   *bool
	upstreamArrayIndex int
}

func newTokenBase() tokenBase {
	return tokenBase{upstreamArrayIndex: -1}
}

func (b *tokenBase) base() *tokenBase {
	return b
}

func appendTailToken(t, next pathToken) pathToken {
	t.base().next = next
	next.base().prev = t
	return next
}

func isLeaf(t pathToken) bool {
	return t.base().next == nil
}

func nextToken(t pathToken) (pathToken, error) {
	if isLeaf(t) {
		return nil, newError(KindIllegalState, "Current path token is a leaf")
	}
	return t.base().next, nil
}

func isUpstreamDefinite(t pathToken) bool {
	b := t.base()
	if b.upstreamDefinite == nil {
		v := b.prev == nil || b.prev.isTokenDefinite() && isUpstreamDefinite(b.prev)
		b.upstreamDefinite = &v
	}
	return *b.upstreamDefinite
}

func isPathDefinite(t pathToken) bool {
	b := t.base()
	if b.definite != nil {
		return *b.definite
	}
	v := t.isTokenDefinite()
	if v && !isLeaf(t) {
		v = isPathDefinite(b.next)
	}
	b.definite = &v
	return v
}

func tokenString(t pathToken) string {
	var sb strings.Builder
	for ; t != nil; t = t.base().next {
		sb.WriteString(t.pathFragment())
	}
	return sb.String()
}

// handleObjectProperty selects one property, or merges several into a new object when the token is a leaf.
func handleObjectProperty(t pathToken, currentPath string, model any, ctx *evalContext, properties []string) error {
	b := t.base()
	if len(properties) == 1 {
		property := properties[0]
		evalPath := currentPath + "['" + property + "']"
		propertyVal := getMapValue(model, property)
		if propertyVal == undefined {
			if isLeaf(t) {
				switch {
				case ctx.has(DefaultPathLeafToNull):
					propertyVal = nil
				case ctx.has(SuppressExceptions) || !ctx.has(RequireProperties):
					return nil
				default:
					return pathNotFound("No results for path: %s", evalPath)
				}
			} else {
				if !(isUpstreamDefinite(t) && t.isTokenDefinite()) && !ctx.has(RequireProperties) ||
					ctx.has(SuppressExceptions) {
					return nil
				}
				return pathNotFound("Missing property in path %s", evalPath)
			}
		}
		ref := noOp
		if ctx.forUpdate {
			ref = &objectPropertyRef{parent: model, property: property}
		}
		if !isLeaf(t) {
			return b.next.evaluate(evalPath, ref, propertyVal, ctx)
		}
		idx := "[" + strconv.Itoa(b.upstreamArrayIndex) + "]"
		if idx != "[-1]" {
			last := ctx.root().tail.base().prev
			if last == nil {
				return &Error{Kind: KindNullPointer}
			}
			if last.pathFragment() != idx {
				return nil
			}
		}
		return ctx.addResult(evalPath, ref, propertyVal)
	}

	quoted := make([]string, len(properties))
	for i, p := range properties {
		quoted[i] = "'" + p + "'"
	}
	evalPath := currentPath + "[" + strings.Join(quoted, ", ") + "]"
	merged := smartjson.NewObject()
	obj := model.(*smartjson.Object)
	for _, property := range properties {
		var propertyVal any
		if v, ok := obj.Get(property); ok {
			propertyVal = v
		} else {
			switch {
			case ctx.has(DefaultPathLeafToNull):
				propertyVal = nil
			case ctx.has(RequireProperties):
				return pathNotFound("Missing property in path %s", evalPath)
			default:
				continue
			}
		}
		merged.Put(property, propertyVal)
	}
	ref := noOp
	if ctx.forUpdate {
		ref = &multiPropertyRef{parent: model, properties: properties}
	}
	return ctx.addResult(evalPath, ref, merged)
}

// handleArrayIndex selects one array element. An index outside the array selects nothing, and so does any index
// failure raised while evaluating the rest of the path below it.
func handleArrayIndex(t pathToken, index int, currentPath string, model any, ctx *evalContext) error {
	evalPath := currentPath + "[" + strconv.Itoa(index) + "]"
	ref := noOp
	if ctx.forUpdate {
		ref = &arrayIndexRef{parent: model, index: index}
	}
	effective := index
	if index < 0 {
		n, err := length(model)
		if err != nil {
			return err
		}
		effective = n + index
	}
	err := func() error {
		hit, err := getArrayIndex(model, effective)
		if err != nil {
			return err
		}
		if isLeaf(t) {
			return ctx.addResult(evalPath, ref, hit)
		}
		return t.base().next.evaluate(evalPath, ref, hit, ctx)
	}()
	if e, ok := err.(*Error); ok && (e.Kind == KindIndexOutOfBounds || e.Kind == KindStringIndexOutOfBounds) {
		return nil
	}
	return err
}

// rootToken is the $ or @ that starts every path.
type rootToken struct {
	tokenBase
	tail       pathToken
	tokenCount int
	symbol     string
}

func newRootToken(symbol uint16) *rootToken {
	r := &rootToken{tokenBase: newTokenBase(), tokenCount: 1, symbol: charString(symbol)}
	r.tail = r
	return r
}

func (r *rootToken) append(next pathToken) {
	r.tail = appendTailToken(r.tail, next)
	r.tokenCount++
}

func (r *rootToken) evaluate(_ string, ref pathRef, model any, ctx *evalContext) error {
	if isLeaf(r) {
		op := noOp
		if ctx.forUpdate {
			op = ref
		}
		return ctx.addResult(r.symbol, op, model)
	}
	return r.next.evaluate(r.symbol, ref, model, ctx)
}

func (r *rootToken) isTokenDefinite() bool {
	return true
}

func (r *rootToken) pathFragment() string {
	return r.symbol
}

func (r *rootToken) isFunctionPath() bool {
	_, ok := r.tail.(*functionToken)
	return ok
}

// propertyToken selects one or more properties: ['a'] or .a, or ['a','b'].
type propertyToken struct {
	tokenBase
	properties []string
	delimiter  string
}

func newPropertyToken(properties []string, delimiter uint16) (*propertyToken, error) {
	if len(properties) == 0 {
		return nil, invalidPath("Empty properties")
	}
	return &propertyToken{tokenBase: newTokenBase(), properties: properties, delimiter: charString(delimiter)}, nil
}

func (p *propertyToken) singlePropertyCase() bool {
	return len(p.properties) == 1
}

func (p *propertyToken) multiPropertyMergeCase() bool {
	return isLeaf(p) && len(p.properties) > 1
}

func (p *propertyToken) evaluate(currentPath string, _ pathRef, model any, ctx *evalContext) error {
	if !isMap(model) {
		if !isUpstreamDefinite(p) || ctx.has(SuppressExceptions) {
			return nil
		}
		found := "null"
		if model != nil {
			found = smartjson.ClassName(model)
		}
		return pathNotFound("Expected to find an object with property %s in path %s but found '%s'. "+
			"This is not a json object according to the JsonProvider: '%s'.",
			p.pathFragment(), currentPath, found, "com.jayway.jsonpath.spi.json.JsonSmartJsonProvider")
	}
	if p.singlePropertyCase() || p.multiPropertyMergeCase() {
		return handleObjectProperty(p, currentPath, model, ctx, p.properties)
	}
	for _, property := range p.properties {
		if err := handleObjectProperty(p, currentPath, model, ctx, []string{property}); err != nil {
			return err
		}
	}
	return nil
}

func (p *propertyToken) isTokenDefinite() bool {
	return p.singlePropertyCase() || p.multiPropertyMergeCase()
}

func (p *propertyToken) pathFragment() string {
	quoted := make([]string, len(p.properties))
	for i, prop := range p.properties {
		quoted[i] = p.delimiter + prop + p.delimiter
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// checkArrayModel decides whether an array token applies to model: false skips it, an error stops the evaluation.
func checkArrayModel(t pathToken, currentPath string, model any, ctx *evalContext) (bool, error) {
	if model == nil {
		if !isUpstreamDefinite(t) || ctx.has(SuppressExceptions) {
			return false, nil
		}
		return false, pathNotFound("The path %s is null", currentPath)
	}
	if !isArray(model) {
		if !isUpstreamDefinite(t) || ctx.has(SuppressExceptions) {
			return false, nil
		}
		return false, pathNotFound("Filter: %s can only be applied to arrays. Current context is: %s",
			tokenString(t), smartjson.ToString(model))
	}
	return true, nil
}

// arrayIndexToken selects elements by index: [0] or [1,-1].
type arrayIndexToken struct {
	tokenBase
	indexes []int
}

func (a *arrayIndexToken) evaluate(currentPath string, _ pathRef, model any, ctx *evalContext) error {
	ok, err := checkArrayModel(a, currentPath, model, ctx)
	if !ok || err != nil {
		return err
	}
	for _, index := range a.indexes {
		if err := handleArrayIndex(a, index, currentPath, model, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (a *arrayIndexToken) pathFragment() string {
	parts := make([]string, len(a.indexes))
	for i, idx := range a.indexes {
		parts[i] = strconv.Itoa(idx)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (a *arrayIndexToken) isTokenDefinite() bool {
	return len(a.indexes) == 1
}

// arraySliceToken selects a range of elements: [1:], [:2] or [1:3].
type arraySliceToken struct {
	tokenBase
	from, to *int
}

func (a *arraySliceToken) evaluate(currentPath string, _ pathRef, model any, ctx *evalContext) error {
	ok, err := checkArrayModel(a, currentPath, model, ctx)
	if !ok || err != nil {
		return err
	}
	n, err := length(model)
	if err != nil {
		return err
	}
	var start, end int
	switch {
	case a.to == nil:
		start = *a.from
		if start < 0 {
			start += n
		}
		start = max(0, start)
		if n == 0 || start >= n {
			return nil
		}
		end = n
	case a.from == nil:
		if n == 0 {
			return nil
		}
		end = *a.to
		if end < 0 {
			end += n
		}
		end = min(n, end)
	default:
		start = *a.from
		end = min(n, *a.to)
		if start >= end || n == 0 {
			return nil
		}
	}
	for i := start; i < end; i++ {
		if err := handleArrayIndex(a, i, currentPath, model, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (a *arraySliceToken) pathFragment() string {
	var sb strings.Builder
	sb.WriteByte('[')
	if a.from != nil {
		sb.WriteString(strconv.Itoa(*a.from))
	}
	sb.WriteByte(':')
	if a.to != nil {
		sb.WriteString(strconv.Itoa(*a.to))
	}
	sb.WriteByte(']')
	return sb.String()
}

func (a *arraySliceToken) isTokenDefinite() bool {
	return false
}

// wildcardToken selects every property or element: * or [*].
type wildcardToken struct {
	tokenBase
}

func newWildcardToken() *wildcardToken {
	return &wildcardToken{tokenBase: newTokenBase()}
}

func (w *wildcardToken) evaluate(currentPath string, _ pathRef, model any, ctx *evalContext) error {
	switch m := model.(type) {
	case *smartjson.Object:
		for _, property := range append([]string(nil), m.Keys()...) {
			if err := handleObjectProperty(w, currentPath, model, ctx, []string{property}); err != nil {
				return err
			}
		}
	case *smartjson.Array:
		for idx := 0; idx < m.Len(); idx++ {
			err := handleArrayIndex(w, idx, currentPath, model, ctx)
			if err != nil && (!isKind(err, ErrPathNotFound) || ctx.has(RequireProperties)) {
				return err
			}
		}
	}
	return nil
}

func (w *wildcardToken) isTokenDefinite() bool {
	return false
}

func (w *wildcardToken) pathFragment() string {
	return "[*]"
}

// predicateToken keeps the elements, or the object, that satisfy its filters: [?(...)] or [?].
type predicateToken struct {
	tokenBase
	predicates []Predicate
}

func (p *predicateToken) evaluate(currentPath string, ref pathRef, model any, ctx *evalContext) error {
	switch m := model.(type) {
	case *smartjson.Object:
		ok, err := p.accept(model, ctx)
		if !ok || err != nil {
			return err
		}
		op := noOp
		if ctx.forUpdate {
			op = ref
		}
		if isLeaf(p) {
			return ctx.addResult(currentPath, op, model)
		}
		return p.next.evaluate(currentPath, op, model, ctx)
	case *smartjson.Array:
		for idx, item := range append([]any(nil), m.Elems()...) {
			ok, err := p.accept(item, ctx)
			if err != nil {
				return err
			}
			if ok {
				if err := handleArrayIndex(p, idx, currentPath, model, ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if isUpstreamDefinite(p) {
		return invalidPath("Filter: %s can not be applied to primitives. Current context is: %s", tokenString(p),
			smartjson.ToString(model))
	}
	return nil
}

// accept applies every filter to item; a filter that fails with an invalid path rejects the item.
func (p *predicateToken) accept(item any, ctx *evalContext) (bool, error) {
	pctx := &PredicateContext{item: item, root: ctx.rootDocument, conf: ctx.conf, cache: ctx.documentEvalCache}
	for _, predicate := range p.predicates {
		ok, err := predicate.Apply(pctx)
		if err != nil {
			if isKind(err, ErrInvalidPath) {
				return false, nil
			}
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (p *predicateToken) pathFragment() string {
	marks := make([]string, len(p.predicates))
	for i := range marks {
		marks[i] = "?"
	}
	return "[" + strings.Join(marks, ",") + "]"
}

func (p *predicateToken) isTokenDefinite() bool {
	return false
}

// functionToken applies a path function to what the path selected so far: .length(), .sum(...).
type functionToken struct {
	tokenBase
	name     string
	fragment string
	params   []*parameter
}

func newFunctionToken(name string, params []*parameter) *functionToken {
	fragment := name + "()"
	if len(params) > 0 {
		fragment = name + "(...)"
	}
	return &functionToken{tokenBase: newTokenBase(), name: name, fragment: fragment, params: params}
}

func (f *functionToken) evaluate(currentPath string, parent pathRef, model any, ctx *evalContext) error {
	fn, err := newPathFunction(f.name)
	if err != nil {
		return err
	}
	if err := f.evaluateParameters(ctx); err != nil {
		return err
	}
	result, err := fn(currentPath, parent, model, ctx, f.params)
	if err != nil {
		return err
	}
	if err := ctx.addResult(currentPath+"."+f.name, parent, result); err != nil {
		return err
	}
	f.cleanWildcardPathToken()
	if !isLeaf(f) {
		return f.next.evaluate(currentPath, parent, result, ctx)
	}
	return nil
}

// cleanWildcardPathToken drops the first wildcard behind the root of the first parameter's path, undoing the
// wildcard that length() appends to its argument.
func (f *functionToken) cleanWildcardPathToken() {
	if len(f.params) == 0 {
		return
	}
	path := f.params[0].path
	if path == nil || path.isFunctionPath() {
		return
	}
	tail := path.root.next
	for tail != nil && tail.base().next != nil {
		if next, ok := tail.base().next.(*wildcardToken); ok {
			tail.base().next = next.next
			break
		}
		tail = tail.base().next
	}
}

func (f *functionToken) evaluateParameters(ctx *evalContext) error {
	for _, param := range f.params {
		switch param.kind {
		case paramPath:
			binding, err := newPathBinding(param.path, ctx.rootDocument, ctx.conf)
			if err != nil {
				return err
			}
			if !param.evaluated || !binding.equals(param.binding) {
				param.binding = binding
				param.evaluated = true
			}
		case paramJSON:
			if !param.evaluated {
				param.binding = &jsonBinding{json: param.json}
				param.evaluated = true
			}
		}
	}
	return nil
}

func (f *functionToken) isTokenDefinite() bool {
	return true
}

func (f *functionToken) pathFragment() string {
	return "." + f.fragment
}

// scanToken walks the whole subtree for whatever the next token selects: ..
type scanToken struct {
	tokenBase
}

func (s *scanToken) evaluate(currentPath string, parent pathRef, model any, ctx *evalContext) error {
	pt, err := nextToken(s)
	if err != nil {
		return err
	}
	return walk(pt, currentPath, parent, model, ctx, scanPredicate(pt, ctx))
}

func (s *scanToken) isTokenDefinite() bool {
	return false
}

func (s *scanToken) pathFragment() string {
	return ".."
}

// scanMatch decides whether the token after a scan applies at a visited node.
type scanMatch func(model any) (bool, error)

func scanPredicate(target pathToken, ctx *evalContext) scanMatch {
	switch t := target.(type) {
	case *propertyToken:
		return func(model any) (bool, error) {
			obj, ok := model.(*smartjson.Object)
			if !ok {
				return false, nil
			}
			if !t.isTokenDefinite() {
				return true, nil
			}
			if isLeaf(t) && ctx.has(DefaultPathLeafToNull) {
				return true, nil
			}
			for _, p := range t.properties {
				if _, ok := obj.Get(p); !ok {
					return false, nil
				}
			}
			return true, nil
		}
	case *arrayIndexToken, *arraySliceToken:
		return func(model any) (bool, error) { return isArray(model), nil }
	case *wildcardToken:
		return func(any) (bool, error) { return true, nil }
	case *predicateToken:
		return func(model any) (bool, error) { return t.accept(model, ctx) }
	}
	return func(any) (bool, error) { return false, nil }
}

func walk(pt pathToken, currentPath string, parent pathRef, model any, ctx *evalContext, match scanMatch) error {
	switch m := model.(type) {
	case *smartjson.Object:
		return walkObject(pt, currentPath, parent, m, ctx, match)
	case *smartjson.Array:
		return walkArray(pt, currentPath, parent, m, ctx, match)
	}
	return nil
}

func walkArray(pt pathToken, currentPath string, parent pathRef, model *smartjson.Array, ctx *evalContext,
	match scanMatch) error {
	ok, err := match(model)
	if err != nil {
		return err
	}
	if ok {
		if isLeaf(pt) {
			if err := pt.evaluate(currentPath, parent, model, ctx); err != nil {
				return err
			}
		} else {
			next := pt.base().next
			for idx, item := range append([]any(nil), model.Elems()...) {
				next.base().upstreamArrayIndex = idx
				if err := next.evaluate(currentPath+"["+strconv.Itoa(idx)+"]", parent, item, ctx); err != nil {
					return err
				}
			}
		}
	}
	for idx, item := range append([]any(nil), model.Elems()...) {
		ref := &arrayIndexRef{parent: model, index: idx}
		if err := walk(pt, currentPath+"["+strconv.Itoa(idx)+"]", ref, item, ctx, match); err != nil {
			return err
		}
	}
	return nil
}

func walkObject(pt pathToken, currentPath string, parent pathRef, model *smartjson.Object, ctx *evalContext,
	match scanMatch) error {
	ok, err := match(model)
	if err != nil {
		return err
	}
	if ok {
		if err := pt.evaluate(currentPath, parent, model, ctx); err != nil {
			return err
		}
	}
	for _, property := range append([]string(nil), model.Keys()...) {
		v, present := model.Get(property)
		if !present {
			continue
		}
		ref := &objectPropertyRef{parent: model, property: property}
		if err := walk(pt, currentPath+"['"+property+"']", ref, v, ctx, match); err != nil {
			return err
		}
	}
	return nil
}
