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

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/smartjson"
)

// compiledPath is a compiled path expression: the chain of tokens below its root.
type compiledPath struct {
	root       *rootToken
	isRootPath bool
}

func newCompiledPath(root *rootToken, isRootPath bool) *compiledPath {
	return &compiledPath{root: invertScannerFunctionRelationship(root), isRootPath: isRootPath}
}

// invertScannerFunctionRelationship rewrites $..path.fn() into $.fn($..path), so that the function receives
// everything the scan finds instead of being applied at every node.
func invertScannerFunctionRelationship(path *rootToken) *rootToken {
	if !path.isFunctionPath() {
		return path
	}
	if _, ok := path.next.(*scanToken); !ok {
		return path
	}
	var token, prior pathToken = path, nil
	for {
		token = token.base().next
		if token == nil {
			return path
		}
		if _, ok := token.(*functionToken); ok {
			break
		}
		prior = token
	}
	fn := token.(*functionToken)
	prior.base().next = nil
	path.tail = prior
	fn.params = []*parameter{{kind: paramPath, path: newCompiledPath(path, true)}}
	functionRoot := newRootToken('$')
	functionRoot.tail = fn
	functionRoot.next = fn
	return functionRoot
}

func (p *compiledPath) isDefinite() bool {
	return isPathDefinite(p.root)
}

func (p *compiledPath) isFunctionPath() bool {
	return p.root.isFunctionPath()
}

// String returns the normalised path.
func (p *compiledPath) String() string {
	return tokenString(p.root)
}

// errAbort stops an evaluation when a listener asks for it; the evaluation then ends normally.
var errAbort = errors.New("evaluation aborted")

func (p *compiledPath) evaluate(document, rootDocument any, conf *Configuration, forUpdate bool) (*evalContext, error) {
	if rootDocument == nil {
		return nil, newError(KindIllegalArgument, "root can not be null")
	}
	ctx := &evalContext{
		conf:              conf,
		path:              p,
		rootDocument:      rootDocument,
		forUpdate:         forUpdate,
		valueResult:       smartjson.NewArray(),
		pathResult:        smartjson.NewArray(),
		documentEvalCache: map[*compiledPath]any{},
	}
	op := noOp
	if forUpdate {
		op = &rootRef{parent: rootDocument}
	}
	if err := p.root.evaluate("", op, document, ctx); err != nil && err != errAbort {
		return nil, err
	}
	return ctx, nil
}

// evalContext collects the results of one evaluation and, for updates, where they live.
type evalContext struct {
	conf              *Configuration
	path              *compiledPath
	rootDocument      any
	forUpdate         bool
	valueResult       *smartjson.Array
	pathResult        *smartjson.Array
	updates           []pathRef
	documentEvalCache map[*compiledPath]any
	resultIndex       int
}

func (c *evalContext) has(o Option) bool {
	return c.conf.Has(o)
}

func (c *evalContext) root() *rootToken {
	return c.path.root
}

func (c *evalContext) addResult(path string, ref pathRef, model any) error {
	if c.forUpdate {
		c.updates = append(c.updates, ref)
	}
	c.valueResult.Set(c.resultIndex, model)
	c.pathResult.Set(c.resultIndex, path)
	c.resultIndex++
	for _, listener := range c.conf.listeners {
		if listener(FoundResult{Index: c.resultIndex - 1, Path: path, Value: model}) == Abort {
			return errAbort
		}
	}
	return nil
}

// value returns the result: the last value found for a definite path, the list of values otherwise.
func (c *evalContext) value() (any, error) {
	if c.path.isDefinite() {
		if c.resultIndex == 0 {
			if c.has(SuppressExceptions) {
				return nil, nil
			}
			return nil, pathNotFound("No results for path: %s", c.path.String())
		}
		if n := c.valueResult.Len(); n > 0 {
			v, _ := c.valueResult.Get(n - 1)
			return v, nil
		}
		return nil, nil
	}
	return c.valueResult, nil
}

// paths returns the list of result paths.
func (c *evalContext) paths() (any, error) {
	if c.resultIndex == 0 {
		if c.has(SuppressExceptions) {
			return nil, nil
		}
		return nil, pathNotFound("No results for path: %s", c.path.String())
	}
	return c.pathResult, nil
}

func (c *evalContext) pathList() []string {
	out := make([]string, 0, c.resultIndex)
	for _, p := range c.pathResult.Elems() {
		out = append(out, p.(string))
	}
	return out
}

// updateOperations returns the locations to update, sorted as JsonPath sorts them: array positions from the highest
// index down, everything else by descending accessor text, so that deleting earlier entries never shifts later ones.
func (c *evalContext) updateOperations() []pathRef {
	sortRefs(c.updates)
	return c.updates
}

// sortRefs sorts like Java's Collections.sort for the short lists updates produce: the leading run is found (and
// reversed when descending), then the rest is placed by binary insertion, which keeps equal elements in order.
func sortRefs(a []pathRef) {
	n := len(a)
	if n < 2 {
		return
	}
	run := 1
	if compareRefs(a[1], a[0]) < 0 {
		run = 2
		for run < n && compareRefs(a[run], a[run-1]) < 0 {
			run++
		}
		for i, j := 0, run-1; i < j; i, j = i+1, j-1 {
			a[i], a[j] = a[j], a[i]
		}
	} else {
		run = 2
		for run < n && compareRefs(a[run], a[run-1]) >= 0 {
			run++
		}
	}
	for start := run; start < n; start++ {
		pivot := a[start]
		left, right := 0, start
		for left < right {
			mid := (left + right) >> 1
			if compareRefs(pivot, a[mid]) < 0 {
				right = mid
			} else {
				left = mid + 1
			}
		}
		copy(a[left+1:start+1], a[left:start])
		a[left] = pivot
	}
}

func compareRefs(a, b pathRef) int {
	ai, aok := a.(*arrayIndexRef)
	bi, bok := b.(*arrayIndexRef)
	if aok && bok {
		switch {
		case bi.index < ai.index:
			return -1
		case bi.index > ai.index:
			return 1
		}
		return 0
	}
	return -java.Compare(a.accessor(), b.accessor())
}
