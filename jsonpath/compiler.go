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
	"regexp"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
)

// pathCompiler turns a path expression into a chain of tokens.
type pathCompiler struct {
	path    *charIndex
	filters []Predicate
}

// compilePath compiles a path expression. The [?] placeholders take the given filters in order. Any failure is an
// invalid path error; failures that are not about the path itself carry the underlying failure as their message.
func compilePath(path string, filters ...Predicate) (p *compiledPath, err error) {
	defer func() {
		if err != nil && !isKind(err, ErrInvalidPath) {
			err = wrap(KindInvalidPath, err)
		}
	}()
	defer recoverIndex(&err)
	ci := newCharIndex(path)
	ci.trim()
	if ci.charAt(0) != '$' && ci.charAt(0) != '@' {
		ci = newCharIndex("$." + path)
		ci.trim()
	}
	if ci.lastCharIs('.') {
		return nil, invalidPath("Path must not end with a '.' or '..'")
	}
	c := &pathCompiler{path: ci, filters: append([]Predicate(nil), filters...)}
	return c.compile()
}

func (c *pathCompiler) compile() (*compiledPath, error) {
	root, err := c.readContextToken()
	if err != nil {
		return nil, err
	}
	return newCompiledPath(root, root.pathFragment() == "$"), nil
}

func isPathWhitespace(c uint16) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (c *pathCompiler) readWhitespace() {
	for c.path.inBoundsHere() && isPathWhitespace(c.path.currentChar()) {
		c.path.incrementPosition(1)
	}
}

func isPathContext(c uint16) bool {
	return c == '$' || c == '@'
}

func (c *pathCompiler) readContextToken() (*rootToken, error) {
	c.readWhitespace()
	if !isPathContext(c.path.currentChar()) {
		return nil, invalidPath("Path must start with '$' or '@'")
	}
	root := newRootToken(c.path.currentChar())
	if c.path.currentIsTail() {
		return root, nil
	}
	c.path.incrementPosition(1)
	if c.path.currentChar() != '.' && c.path.currentChar() != '[' {
		return nil, invalidPath("Illegal character at position %d expected '.' or '['", c.path.position)
	}
	if _, err := c.readNextToken(root); err != nil {
		return nil, err
	}
	return root, nil
}

// readNextToken reads the token at the cursor and everything after it.
func (c *pathCompiler) readNextToken(root *rootToken) (bool, error) {
	ch := c.path.currentChar()
	var ok bool
	var err error
	switch ch {
	case '[':
		for _, read := range []func(*rootToken) (bool, error){
			c.readBracketPropertyToken, c.readArrayToken, c.readWildCardToken, c.readFilterToken,
			c.readPlaceholderToken,
		} {
			if ok, err = read(root); ok || err != nil {
				break
			}
		}
		if err == nil && !ok {
			err = invalidPath("Could not parse token starting at position %d. Expected ?, ', 0-9, * ", c.path.position)
		}
	case '.':
		if ok, err = c.readDotToken(root); err == nil && !ok {
			err = invalidPath("Could not parse token starting at position %d", c.path.position)
		}
	case '*':
		if ok, err = c.readWildCardToken(root); err == nil && !ok {
			err = invalidPath("Could not parse token starting at position %d", c.path.position)
		}
	default:
		if ok, err = c.readPropertyOrFunctionToken(root); err == nil && !ok {
			err = invalidPath("Could not parse token starting at position %d", c.path.position)
		}
	}
	return err == nil, err
}

// continueAfter finishes a token: at the end of the path compilation stops, otherwise the next token is read.
func (c *pathCompiler) continueAfter(root *rootToken) (bool, error) {
	if c.path.currentIsTail() {
		return true, nil
	}
	return c.readNextToken(root)
}

func (c *pathCompiler) readDotToken(root *rootToken) (bool, error) {
	switch {
	case c.path.currentCharIs('.') && c.path.nextCharIs('.'):
		root.append(&scanToken{tokenBase: newTokenBase()})
		c.path.incrementPosition(2)
	case !c.path.hasMoreCharacters():
		return false, invalidPath("Path must not end with a '.")
	default:
		c.path.incrementPosition(1)
	}
	if c.path.currentCharIs('.') {
		return false, invalidPath("Character '.' on position %d is not valid.", c.path.position)
	}
	return c.readNextToken(root)
}

func (c *pathCompiler) readPropertyOrFunctionToken(root *rootToken) (bool, error) {
	p := c.path
	if p.currentCharIs('[') || p.currentCharIs('*') || p.currentCharIs('.') || p.currentCharIs(' ') {
		return false, nil
	}
	start := p.position
	read := start
	end := 0
	isFunction := false
	for p.inBounds(read) {
		ch := p.charAt(read)
		if ch == ' ' {
			return false, invalidPath("Use bracket notion ['my prop'] if your property contains blank characters. position: %d",
				p.position)
		}
		if ch == '.' || ch == '[' {
			end = read
			break
		}
		if ch == '(' {
			isFunction = true
			end = read
			break
		}
		read++
	}
	if end == 0 {
		end = p.length()
	}
	var params []*parameter
	if isFunction {
		depth := 1
		for i := read + 1; i < p.length(); i++ {
			if p.charAt(i) == ')' {
				depth--
			} else if p.charAt(i) == '(' {
				depth++
			}
			if depth == 0 {
				break
			}
		}
		if depth != 0 {
			return false, invalidPath("Arguments to function: '%s' are not closed properly.", p.subSequence(start, end))
		}
		if p.inBounds(read + 1) {
			if p.charAt(read+1) != ')' {
				p.setPosition(end + 1)
				var err error
				if params, err = c.parseFunctionParameters(p.subSequence(start, end)); err != nil {
					return false, err
				}
			} else {
				p.setPosition(read + 1)
			}
		} else {
			p.setPosition(read)
		}
	} else {
		p.setPosition(end)
	}
	property := p.subSequence(start, end)
	if isFunction {
		root.append(newFunctionToken(property, params))
	} else {
		token, err := newPropertyToken([]string{property}, '\'')
		if err != nil {
			return false, err
		}
		root.append(token)
	}
	return c.continueAfter(root)
}

// parseFunctionParameters splits the arguments of a function call at top-level commas: arguments starting with $
// or @ are paths, arguments starting like a JSON value are JSON text, anything else is dropped.
func (c *pathCompiler) parseFunctionParameters(funcName string) ([]*parameter, error) {
	p := c.path
	kind := paramNone
	groupParen, groupBracket, groupBrace, groupQuote := 1, 0, 0, 0
	endOfStream := false
	var prior uint16
	var params []*parameter
	var param java.Chars
	for p.inBoundsHere() && !endOfStream {
		ch := p.currentChar()
		p.incrementPosition(1)
		if kind == paramNone {
			if isPathWhitespace(ch) {
				continue
			}
			if ch == '{' || java.IsDigit(rune(ch)) || ch == '"' || ch == '-' {
				kind = paramJSON
			} else if isPathContext(ch) {
				kind = paramPath
			}
		}
		switch ch {
		case '"':
			if prior != '\\' && groupQuote > 0 {
				groupQuote--
			} else {
				groupQuote++
			}
		case '(':
			groupParen++
		case '{':
			groupBrace++
		case '[':
			groupBracket++
		case '}':
			if groupBrace == 0 {
				return nil, invalidPath("Unexpected close brace '}' at character position: %d", p.position)
			}
			groupBrace--
		case ']':
			if groupBracket == 0 {
				return nil, invalidPath("Unexpected close bracket ']' at character position: %d", p.position)
			}
			groupBracket--
		}
		if ch == ')' {
			groupParen--
			if groupParen < 0 || prior == '(' {
				param = append(param, ch)
			}
		}
		if (ch == ')' || ch == ',') && groupQuote == 0 && groupBrace == 0 && groupBracket == 0 &&
			(groupParen == 0 && ch == ')' || groupParen == 1) {
			endOfStream = groupParen == 0
			if kind != paramNone {
				switch kind {
				case paramJSON:
					params = append(params, &parameter{kind: paramJSON, json: param.String()})
				case paramPath:
					sub := &pathCompiler{path: newCharIndex(param.String())}
					compiled, err := sub.compile()
					if err != nil {
						return nil, err
					}
					params = append(params, &parameter{kind: paramPath, path: compiled})
				}
				param = param[:0]
				kind = paramNone
			}
		}
		if kind != paramNone && !(ch == ',' && groupBrace == 0 && groupBracket == 0 && groupParen == 1) {
			param = append(param, ch)
		}
		prior = ch
	}
	if groupBrace != 0 || groupParen != 0 || groupBracket != 0 {
		return nil, invalidPath("Arguments to function: '%s' are not closed properly.", funcName)
	}
	return params, nil
}

func (c *pathCompiler) readPlaceholderToken(root *rootToken) (bool, error) {
	p := c.path
	if !p.currentCharIs('[') {
		return false, nil
	}
	question := p.indexOfNextSignificantChar('?')
	if question == -1 {
		return false, nil
	}
	next := p.nextSignificantCharFrom(question)
	if next != ']' && next != ',' {
		return false, nil
	}
	begin := p.position + 1
	end := p.nextIndexOf(begin, ']')
	if end == -1 {
		return false, nil
	}
	expression := p.subSequence(begin, end)
	tokens := javaSplit(expression, ",")
	if len(c.filters) < len(tokens) {
		return false, invalidPath("Not enough predicates supplied for filter [%s] at position %d", expression, p.position)
	}
	var predicates []Predicate
	for _, token := range tokens {
		token = java.Trim(token)
		if token != "?" {
			return false, invalidPath("Expected '?' but found %s", token)
		}
		predicates = append(predicates, c.filters[0])
		c.filters = c.filters[1:]
	}
	root.append(&predicateToken{tokenBase: newTokenBase(), predicates: predicates})
	p.setPosition(end + 1)
	return c.continueAfter(root)
}

// javaSplit splits like Java's String.split(regex): trailing empty strings are removed.
func javaSplit(s, sep string) []string {
	parts := strings.Split(s, sep)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		// Splitting text made only of separators yields no parts; splitting "" yields [""].
		if s == "" {
			return []string{""}
		}
		return nil
	}
	return parts
}

func (c *pathCompiler) readFilterToken(root *rootToken) (bool, error) {
	p := c.path
	if !p.currentCharIs('[') && !p.nextSignificantCharIs('?') {
		return false, nil
	}
	openStatement := p.position
	question := p.indexOfNextSignificantChar('?')
	if question == -1 {
		return false, nil
	}
	openBracket := p.indexOfNextSignificantCharFrom(question, '(')
	if openBracket == -1 {
		return false, nil
	}
	closeBracket, err := p.indexOfClosingBracket(openBracket, true, true)
	if err != nil {
		return false, err
	}
	if closeBracket == -1 {
		return false, nil
	}
	if !p.nextSignificantCharIsFrom(closeBracket, ']') {
		return false, nil
	}
	closeStatement := p.indexOfNextSignificantCharFrom(closeBracket, ']')
	criteria := p.subSequence(openStatement, closeStatement+1)
	filter, err := compileFilter(criteria)
	if err != nil {
		return false, err
	}
	root.append(&predicateToken{tokenBase: newTokenBase(), predicates: []Predicate{filter}})
	p.setPosition(closeStatement + 1)
	return c.continueAfter(root)
}

func (c *pathCompiler) readWildCardToken(root *rootToken) (bool, error) {
	p := c.path
	inBracket := p.currentCharIs('[')
	if inBracket && !p.nextSignificantCharIs('*') {
		return false, nil
	}
	if !p.currentCharIs('*') && p.isOutOfBounds(p.position+1) {
		return false, nil
	}
	if inBracket {
		wildcard := p.indexOfNextSignificantChar('*')
		if !p.nextSignificantCharIsFrom(wildcard, ']') {
			return false, invalidPath("Expected wildcard token to end with ']' on position %d", wildcard+1)
		}
		closing := p.indexOfNextSignificantCharFrom(wildcard, ']')
		p.setPosition(closing + 1)
	} else {
		p.incrementPosition(1)
	}
	root.append(newWildcardToken())
	return c.continueAfter(root)
}

var commaSplit = regexp.MustCompile(`\s*,\s*`)

func (c *pathCompiler) readArrayToken(root *rootToken) (bool, error) {
	p := c.path
	if !p.currentCharIs('[') {
		return false, nil
	}
	next := p.nextSignificantChar()
	if !java.IsDigit(rune(next)) && next != '-' && next != ':' {
		return false, nil
	}
	begin := p.position + 1
	end := p.nextIndexOf(begin, ']')
	if end == -1 {
		return false, nil
	}
	expression := java.Trim(p.subSequence(begin, end))
	if expression == "*" {
		return false, nil
	}
	for _, ch := range java.ToChars(expression) {
		if !java.IsDigit(rune(ch)) && ch != ',' && ch != '-' && ch != ':' && ch != ' ' {
			return false, nil
		}
	}
	var token pathToken
	var err error
	if strings.Contains(expression, ":") {
		token, err = parseSlice(expression)
	} else {
		token, err = parseIndexes(expression)
	}
	if err != nil {
		return false, err
	}
	root.append(token)
	p.setPosition(end + 1)
	return c.continueAfter(root)
}

func parseIndexes(operation string) (*arrayIndexToken, error) {
	for _, ch := range java.ToChars(operation) {
		if !java.IsDigit(rune(ch)) && ch != ',' && ch != ' ' && ch != '-' {
			return nil, invalidPath("Failed to parse ArrayIndexOperation: %s", operation)
		}
	}
	var indexes []int
	for _, token := range commaSplit.Split(operation, -1) {
		v, err := java.ParseInt(token)
		if err != nil {
			return nil, invalidPath("Failed to parse token in ArrayIndexOperation: %s", token)
		}
		indexes = append(indexes, int(v))
	}
	return &arrayIndexToken{tokenBase: newTokenBase(), indexes: indexes}, nil
}

func parseSlice(operation string) (*arraySliceToken, error) {
	for _, ch := range java.ToChars(operation) {
		if !java.IsDigit(rune(ch)) && ch != '-' && ch != ':' {
			return nil, invalidPath("Failed to parse SliceOperation: %s", operation)
		}
	}
	tokens := javaSplit(operation, ":")
	read := func(i int) (*int, error) {
		if len(tokens) <= i || tokens[i] == "" {
			return nil, nil
		}
		v, err := java.ParseInt(tokens[i])
		if err != nil {
			return nil, err
		}
		n := int(v)
		return &n, nil
	}
	from, err := read(0)
	if err != nil {
		return nil, err
	}
	to, err := read(1)
	if err != nil {
		return nil, err
	}
	if from == nil && to == nil {
		return nil, invalidPath("Failed to parse SliceOperation: %s", operation)
	}
	return &arraySliceToken{tokenBase: newTokenBase(), from: from, to: to}, nil
}

func (c *pathCompiler) readBracketPropertyToken(root *rootToken) (bool, error) {
	p := c.path
	if !p.currentCharIs('[') {
		return false, nil
	}
	delimiter := p.nextSignificantChar()
	if delimiter != '\'' && delimiter != '"' {
		return false, nil
	}
	var properties []string
	start := p.position + 1
	read := start
	end := 0
	inProperty, inEscape, lastWasComma := false, false, false
scan:
	for p.inBounds(read) {
		ch := p.charAt(read)
		switch {
		case inEscape:
			inEscape = false
		case ch == '\\':
			inEscape = true
		case ch == ']' && !inProperty:
			if lastWasComma {
				return false, invalidPath("Found empty property at index %d", read)
			}
			break scan
		case ch == delimiter:
			if inProperty {
				next := p.nextSignificantCharFrom(read)
				if next != ']' && next != ',' {
					return false, invalidPath("Property must be separated by comma or Property must be terminated close square bracket at index %d", read)
				}
				end = read
				prop, err := unescape(p.subSequence(start, end))
				if err != nil {
					return false, err
				}
				properties = append(properties, prop)
				inProperty = false
			} else {
				start = read + 1
				inProperty = true
				lastWasComma = false
			}
		case ch == ',' && !inProperty:
			if lastWasComma {
				return false, invalidPath("Found empty property at index %d", read)
			}
			lastWasComma = true
		}
		read++
	}
	if inProperty {
		return false, invalidPath("Property has not been closed - missing closing %s", charString(delimiter))
	}
	closing := p.indexOfNextSignificantCharFrom(end, ']')
	if closing == -1 {
		return false, invalidPath("Property has not been closed - missing closing ]")
	}
	p.setPosition(closing + 1)
	token, err := newPropertyToken(properties, delimiter)
	if err != nil {
		return false, err
	}
	root.append(token)
	return c.continueAfter(root)
}

// unescape resolves the backslash escapes of a quoted property or string literal, as JsonPath's Utils.unescape.
func unescape(s string) (string, error) {
	var out java.Chars
	var unicode java.Chars
	hadSlash, inUnicode := false, false
	for _, ch := range java.ToChars(s) {
		if inUnicode {
			unicode = append(unicode, ch)
			if len(unicode) == 4 {
				v, ok := parseHexUnit(unicode)
				if !ok {
					return "", newError(KindJSONPath, "Unable to parse unicode value: %s", unicode.String())
				}
				out = append(out, v)
				unicode = unicode[:0]
				inUnicode, hadSlash = false, false
			}
			continue
		}
		if hadSlash {
			hadSlash = false
			switch ch {
			case 'r':
				out = append(out, '\r')
			case 'f':
				out = append(out, '\f')
			case 't':
				out = append(out, '\t')
			case 'n':
				out = append(out, '\n')
			case 'b':
				out = append(out, '\b')
			case 'u':
				inUnicode = true
			default:
				out = append(out, ch)
			}
			continue
		}
		if ch == '\\' {
			hadSlash = true
			continue
		}
		out = append(out, ch)
	}
	if hadSlash {
		out = append(out, '\\')
	}
	return out.String(), nil
}

// parseHexUnit reads four characters as Integer.parseInt(s, 16) and narrows the value to a UTF-16 unit; a leading
// sign is allowed as in Java.
func parseHexUnit(s java.Chars) (uint16, bool) {
	neg := false
	digits := s
	if digits[0] == '-' || digits[0] == '+' {
		neg = digits[0] == '-'
		digits = digits[1:]
	}
	v := 0
	for _, c := range digits {
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		v = v*16 + d
	}
	if neg {
		v = -v
	}
	return uint16(v), true
}
