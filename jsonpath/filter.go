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
	"strings"

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/internal/javaregex"
)

// Predicate decides whether an item passes a filter.
type Predicate interface {
	// Apply reports whether the item in ctx passes; an error of kind KindInvalidPath rejects the item.
	Apply(ctx *PredicateContext) (bool, error)
	// String returns the predicate in JsonPath's notation.
	String() string
}

// PredicateContext is what a predicate sees: the item under test, the document root and the configuration.
type PredicateContext struct {
	item  any
	root  any
	conf  *Configuration
	cache map[*compiledPath]any
}

// Item returns the item under test.
func (c *PredicateContext) Item() any {
	return c.item
}

// Root returns the document being evaluated.
func (c *PredicateContext) Root() any {
	return c.root
}

// Configuration returns the configuration of the evaluation.
func (c *PredicateContext) Configuration() *Configuration {
	return c.conf
}

// evaluate evaluates a path inside a filter: $-paths against the document (once per evaluation), others against
// the item.
func (c *PredicateContext) evaluate(path *compiledPath) (any, error) {
	if path.isRootPath {
		if v, ok := c.cache[path]; ok {
			return v, nil
		}
		ec, err := path.evaluate(c.root, c.root, c.conf, false)
		if err != nil {
			return nil, err
		}
		v, err := ec.value()
		if err != nil {
			return nil, err
		}
		c.cache[path] = v
		return v, nil
	}
	ec, err := path.evaluate(c.item, c.root, c.conf, false)
	if err != nil {
		return nil, err
	}
	return ec.value()
}

// Filter is a compiled inline filter such as [?(@.price < 10)].
type Filter struct {
	predicate expressionNode
}

// ParseFilter compiles a filter expression of the form [?(<expression>)].
func ParseFilter(text string) (*Filter, error) {
	return compileFilter(text)
}

// Apply evaluates the filter for the item in ctx.
func (f *Filter) Apply(ctx *PredicateContext) (bool, error) {
	return f.predicate.Apply(ctx)
}

// String returns the filter in JsonPath's normalised notation.
func (f *Filter) String() string {
	s := f.predicate.String()
	if strings.HasPrefix(s, "(") {
		return "[?" + s + "]"
	}
	return "[?(" + s + ")]"
}

// expressionNode is a node of a filter expression tree.
type expressionNode interface {
	Predicate
}

type logicalOperator string

const (
	opAnd logicalOperator = "&&"
	opNot logicalOperator = "!"
	opOr  logicalOperator = "||"
)

// logicalNode combines expressions; a NOT node holds its operand and a nil second element, as JsonPath does,
// which shows in its text.
type logicalNode struct {
	operator logicalOperator
	chain    []expressionNode
}

// String joins the operands with the operator inside parentheses.
func (n *logicalNode) String() string {
	parts := make([]string, len(n.chain))
	for i, e := range n.chain {
		if e == nil {
			parts[i] = "null"
		} else {
			parts[i] = e.String()
		}
	}
	return "(" + strings.Join(parts, " "+string(n.operator)+" ") + ")"
}

// Apply evaluates the operands left to right, stopping as soon as the result is known.
func (n *logicalNode) Apply(ctx *PredicateContext) (bool, error) {
	switch n.operator {
	case opOr:
		for _, e := range n.chain {
			ok, err := e.Apply(ctx)
			if err != nil || ok {
				return ok, err
			}
		}
		return false, nil
	case opAnd:
		for _, e := range n.chain {
			ok, err := e.Apply(ctx)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	}
	ok, err := n.chain[0].Apply(ctx)
	return !ok && err == nil, err
}

// relationalNode compares two operands.
type relationalNode struct {
	left     valueNode
	operator relationalOperator
	right    valueNode
}

// String renders the comparison; an existence check is just its path.
func (n *relationalNode) String() string {
	if n.operator == opExists {
		return n.left.String()
	}
	return n.left.String() + " " + string(n.operator) + " " + n.right.String()
}

// Apply evaluates path operands for the item and applies the operator's evaluator.
func (n *relationalNode) Apply(ctx *PredicateContext) (bool, error) {
	l, r := n.left, n.right
	var err error
	if p, ok := l.(*pathNode); ok {
		if l, err = p.evaluate(ctx); err != nil {
			return false, err
		}
	}
	if p, ok := r.(*pathNode); ok {
		if r, err = p.evaluate(ctx); err != nil {
			return false, err
		}
	}
	eval, ok := evaluators[n.operator]
	if !ok {
		return false, nil
	}
	return eval(l, r)
}

type relationalOperator string

const (
	opGTE      relationalOperator = ">="
	opLTE      relationalOperator = "<="
	opEQ       relationalOperator = "=="
	opTSEQ     relationalOperator = "==="
	opNE       relationalOperator = "!="
	opTSNE     relationalOperator = "!=="
	opLT       relationalOperator = "<"
	opGT       relationalOperator = ">"
	opRegex    relationalOperator = "=~"
	opNIN      relationalOperator = "NIN"
	opIN       relationalOperator = "IN"
	opContains relationalOperator = "CONTAINS"
	opAll      relationalOperator = "ALL"
	opSize     relationalOperator = "SIZE"
	opExists   relationalOperator = "EXISTS"
	opType     relationalOperator = "TYPE"
	opMatches  relationalOperator = "MATCHES"
	opEmpty    relationalOperator = "EMPTY"
	opSubsetOf relationalOperator = "SUBSETOF"
	opAnyOf    relationalOperator = "ANYOF"
	opNoneOf   relationalOperator = "NONEOF"
)

var relationalOperators = []relationalOperator{
	opGTE, opLTE, opEQ, opTSEQ, opNE, opTSNE, opLT, opGT, opRegex, opNIN, opIN, opContains, opAll, opSize,
	opExists, opType, opMatches, opEmpty, opSubsetOf, opAnyOf, opNoneOf,
}

func parseRelationalOperator(s string) (relationalOperator, error) {
	upper := java.ToUpperRoot(s)
	for _, op := range relationalOperators {
		if string(op) == upper {
			return op, nil
		}
	}
	return "", invalidPath("Filter operator %s is not supported!", s)
}

// filterCompiler parses the expression inside [?( ... )].
type filterCompiler struct {
	filter *charIndex
}

// compileFilter compiles a filter. Failures other than invalid-path ones are reported with the position where
// parsing stopped; a position past the end makes that report itself fail with a string index error.
func compileFilter(text string) (f *Filter, err error) {
	defer recoverIndex(&err)
	ci := newCharIndex(text)
	ci.trim()
	if !ci.currentCharIs('[') || !ci.lastCharIs(']') {
		return nil, invalidPath("Filter must start with '[' and end with ']'. %s", text)
	}
	ci.incrementPosition(1)
	ci.decrementEndPosition(1)
	ci.trim()
	if !ci.currentCharIs('?') {
		return nil, invalidPath("Filter must start with '[?' and end with ']'. %s", text)
	}
	ci.incrementPosition(1)
	ci.trim()
	if !ci.currentCharIs('(') || !ci.lastCharIs(')') {
		return nil, invalidPath("Filter must start with '[?(' and end with ')]'. %s", text)
	}
	c := &filterCompiler{filter: ci}
	node, err := c.compile()
	if err != nil {
		return nil, err
	}
	return &Filter{predicate: node}, nil
}

func (c *filterCompiler) compile() (node expressionNode, err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(indexFault); !ok {
				panic(r)
			}
			err = c.parseFailure()
		}
	}()
	node, err = c.readLogicalOR()
	if err == nil {
		c.filter.skipBlanks()
		if c.filter.inBoundsHere() {
			err = invalidPath("Expected end of filter expression instead of: %s",
				c.filter.subSequence(c.filter.position, c.filter.length()))
		}
	}
	if err != nil && !isKind(err, ErrInvalidPath) {
		err = c.parseFailure()
	}
	return node, err
}

// parseFailure reports an unexpected failure; reading the current character may itself fail past the end.
func (c *filterCompiler) parseFailure() error {
	return invalidPath("Failed to parse filter: %s, error on position: %d, char: %s", c.filter.String(),
		c.filter.position, charString(c.filter.currentChar()))
}

func (c *filterCompiler) readValueNode() (valueNode, error) {
	switch c.filter.skipBlanks().currentChar() {
	case '$', '@':
		return c.readPath()
	case '!':
		c.filter.incrementPosition(1)
		switch c.filter.skipBlanks().currentChar() {
		case '$', '@':
			return c.readPath()
		}
		return nil, invalidPath("Unexpected character: !")
	}
	return c.readLiteral()
}

func (c *filterCompiler) readLiteral() (valueNode, error) {
	switch c.filter.skipBlanks().currentChar() {
	case '\'':
		return c.readStringLiteral('\'')
	case '"':
		return c.readStringLiteral('"')
	case 't', 'f':
		return c.readBooleanLiteral()
	case '-':
		return c.readNumberLiteral()
	case 'n':
		return c.readNullLiteral()
	case '{', '[':
		return c.readJSONLiteral()
	case '/':
		return c.readPattern()
	}
	return c.readNumberLiteral()
}

func (c *filterCompiler) readLogicalOR() (expressionNode, error) {
	first, err := c.readLogicalAND()
	if err != nil {
		return nil, err
	}
	ops := []expressionNode{first}
	for {
		save := c.filter.position
		if !c.filter.hasSignificantSubSequence(string(opOr)) {
			c.filter.setPosition(save)
			break
		}
		next, err := c.readLogicalAND()
		if err != nil {
			return nil, err
		}
		ops = append(ops, next)
	}
	if len(ops) == 1 {
		return ops[0], nil
	}
	return &logicalNode{operator: opOr, chain: ops}, nil
}

func (c *filterCompiler) readLogicalAND() (expressionNode, error) {
	first, err := c.readLogicalANDOperand()
	if err != nil {
		return nil, err
	}
	ops := []expressionNode{first}
	for {
		save := c.filter.position
		if !c.filter.hasSignificantSubSequence(string(opAnd)) {
			c.filter.setPosition(save)
			break
		}
		next, err := c.readLogicalANDOperand()
		if err != nil {
			return nil, err
		}
		ops = append(ops, next)
	}
	if len(ops) == 1 {
		return ops[0], nil
	}
	return &logicalNode{operator: opAnd, chain: ops}, nil
}

func (c *filterCompiler) readLogicalANDOperand() (expressionNode, error) {
	save := c.filter.skipBlanks().position
	if c.filter.skipBlanks().currentCharIs('!') {
		if err := c.filter.readSignificantChar('!'); err != nil {
			return nil, err
		}
		switch c.filter.skipBlanks().currentChar() {
		case '$', '@':
			c.filter.setPosition(save)
		default:
			op, err := c.readLogicalANDOperand()
			if err != nil {
				return nil, err
			}
			return &logicalNode{operator: opNot, chain: []expressionNode{op, nil}}, nil
		}
	}
	if c.filter.skipBlanks().currentCharIs('(') {
		if err := c.filter.readSignificantChar('('); err != nil {
			return nil, err
		}
		op, err := c.readLogicalOR()
		if err != nil {
			return nil, err
		}
		if err := c.filter.readSignificantChar(')'); err != nil {
			return nil, err
		}
		return op, nil
	}
	return c.readExpression()
}

func (c *filterCompiler) readExpression() (expressionNode, error) {
	left, err := c.readValueNode()
	if err != nil {
		return nil, err
	}
	save := c.filter.position
	op, err := c.readRelationalOperator()
	if err == nil {
		var right valueNode
		if right, err = c.readValueNode(); err == nil {
			return &relationalNode{left: left, operator: op, right: right}, nil
		}
	}
	if !isKind(err, ErrInvalidPath) {
		return nil, err
	}
	c.filter.setPosition(save)
	path, ok := left.(*pathNode)
	if !ok {
		return nil, invalidPath("Expected path node")
	}
	exists := path.asExistsCheck(path.shouldExist)
	right := falseValue
	if exists.shouldExist {
		right = trueValue
	}
	return &relationalNode{left: exists, operator: opExists, right: right}, nil
}

func isRelationalOperatorChar(c uint16) bool {
	return c == '<' || c == '>' || c == '=' || c == '~' || c == '!'
}

func (c *filterCompiler) readRelationalOperator() (relationalOperator, error) {
	f := c.filter
	begin := f.skipBlanks().position
	if isRelationalOperatorChar(f.currentChar()) {
		for f.inBoundsHere() && isRelationalOperatorChar(f.currentChar()) {
			f.incrementPosition(1)
		}
	} else {
		for f.inBoundsHere() && f.currentChar() != ' ' {
			f.incrementPosition(1)
		}
	}
	return parseRelationalOperator(f.subSequence(begin, f.position))
}

func (c *filterCompiler) readNullLiteral() (valueNode, error) {
	f := c.filter
	if f.currentChar() == 'n' && f.inBounds(f.position+3) && f.subSequence(f.position, f.position+4) == "null" {
		f.incrementPosition(4)
		return nullValue, nil
	}
	return nil, invalidPath("Expected <null> value")
}

func (c *filterCompiler) readJSONLiteral() (valueNode, error) {
	f := c.filter
	begin := f.position
	open := f.currentChar()
	closeChar := uint16(']')
	if open == '{' {
		closeChar = '}'
	}
	closing, err := f.indexOfMatchingCloseChar(f.position, open, closeChar, true, false)
	if err != nil {
		return nil, err
	}
	if closing == -1 {
		return nil, invalidPath("String not closed. Expected ' in %s", f.String())
	}
	f.setPosition(closing + 1)
	return &jsonNode{text: f.subSequence(begin, f.position)}, nil
}

func (c *filterCompiler) endOfFlags(position int) int {
	end := position
	for c.filter.inBounds(end) && javaregex.FlagOf(rune(c.filter.charAt(end))) > 0 {
		end++
	}
	return end
}

func (c *filterCompiler) readPattern() (valueNode, error) {
	f := c.filter
	begin := f.position
	closing := f.nextIndexOfUnescaped(f.position, '/')
	if closing == -1 {
		return nil, invalidPath("Pattern not closed. Expected / in %s", f.String())
	}
	if f.inBounds(closing + 1) {
		closing = c.endOfFlags(closing+1) - 1
	}
	f.setPosition(closing + 1)
	return newPatternNode(f.subSequence(begin, f.position))
}

func (c *filterCompiler) readStringLiteral(end uint16) (valueNode, error) {
	f := c.filter
	begin := f.position
	closing := f.nextIndexOfUnescaped(f.position, end)
	if closing == -1 {
		return nil, invalidPath("String literal does not have matching quotes. Expected %s in %s", charString(end),
			f.String())
	}
	f.setPosition(closing + 1)
	return newStringNode(f.subSequence(begin, f.position), true)
}

func (c *filterCompiler) readNumberLiteral() (valueNode, error) {
	f := c.filter
	begin := f.position
	for f.inBoundsHere() && f.isNumberCharacter(f.position) {
		f.incrementPosition(1)
	}
	return newNumberNode(f.subSequence(begin, f.position))
}

func (c *filterCompiler) readBooleanLiteral() (valueNode, error) {
	f := c.filter
	begin := f.position
	end := f.position + 4
	if f.currentChar() == 't' {
		end = f.position + 3
	}
	if !f.inBounds(end) {
		return nil, invalidPath("Expected boolean literal")
	}
	value := f.subSequence(begin, end+1)
	if value != "true" && value != "false" {
		return nil, invalidPath("Expected boolean literal")
	}
	f.incrementPosition(java.Len(value))
	return boolNode(value == "true"), nil
}

func (c *filterCompiler) readPath() (valueNode, error) {
	f := c.filter
	previous := f.previousSignificantChar()
	begin := f.position
	f.incrementPosition(1)
	for f.inBoundsHere() {
		if f.currentChar() == '[' {
			closing, err := f.indexOfMatchingCloseChar(f.position, '[', ']', true, false)
			if err != nil {
				return nil, err
			}
			if closing == -1 {
				return nil, invalidPath("Square brackets does not match in filter %s", f.String())
			}
			f.setPosition(closing + 1)
		}
		closingFunction := f.currentChar() == ')' && c.currentCharIsClosingFunctionBracket(begin)
		closingLogical := f.currentChar() == ')' && !closingFunction
		if !f.inBoundsHere() || isRelationalOperatorChar(f.currentChar()) || f.currentChar() == ' ' || closingLogical {
			break
		}
		f.incrementPosition(1)
	}
	return newPathNode(f.subSequence(begin, f.position), false, previous != '!')
}

func (c *filterCompiler) currentCharIsClosingFunctionBracket(lowerBound int) bool {
	f := c.filter
	if f.currentChar() != ')' {
		return false
	}
	idx := f.indexOfPreviousSignificantCharFrom(f.position)
	if idx == -1 || f.charAt(idx) != '(' {
		return false
	}
	idx--
	for f.inBounds(idx) && idx > lowerBound {
		if f.charAt(idx) == '.' {
			return true
		}
		idx--
	}
	return false
}
