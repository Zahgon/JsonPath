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

package smartjson

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
)

// Error kinds of a ParseError, with json-smart's numbering.
const (
	ErrUnexpectedChar = iota
	ErrUnexpectedToken
	ErrUnexpectedException
	ErrUnexpectedEOF
	ErrUnexpectedUnicode
	ErrUnexpectedDuplicateKey
	ErrUnexpectedLeadingZero
	ErrUnexpectedJSONDepth
)

// ParseError is a syntax error found by the parser. Position counts UTF-16 code units from 0.
type ParseError struct {
	Position   int
	Kind       int
	Unexpected string
}

// Error returns json-smart's message for the error, with its position.
func (e *ParseError) Error() string {
	switch e.Kind {
	case ErrUnexpectedChar:
		return fmt.Sprintf("Unexpected character (%s) at position %d.", e.Unexpected, e.Position)
	case ErrUnexpectedToken:
		return fmt.Sprintf("Unexpected token %s at position %d.", e.Unexpected, e.Position)
	case ErrUnexpectedEOF:
		return fmt.Sprintf("Unexpected End Of File position %d: %s", e.Position, e.Unexpected)
	case ErrUnexpectedUnicode:
		return fmt.Sprintf("Unexpected unicode escape sequence %s at position %d.", e.Unexpected, e.Position)
	case ErrUnexpectedLeadingZero:
		return fmt.Sprintf("Unexpected leading 0 in digit for token:%s at position %d.", e.Unexpected, e.Position)
	case ErrUnexpectedJSONDepth:
		return fmt.Sprintf("Malicious payload, having non natural depths, parsing stoped on %s at position %d.",
			e.Unexpected, e.Position)
	}
	return fmt.Sprintf("Unkown error at position %d.", e.Position)
}

// String is the Java form of the exception, class name first, as wrapping exceptions quote it.
func (e *ParseError) String() string {
	return "net.minidev.json.parser.ParseException: " + e.Error()
}

const (
	eoi       = 0x1a
	maxStop   = 126
	maxDepth  = 400
	nullToken = "null"
)

type stopSet [maxStop]bool

var stopAll, stopArray, stopKey, stopValue, stopX stopSet

func init() {
	stopKey[':'], stopKey[eoi] = true, true
	stopValue[','], stopValue['}'], stopValue[eoi] = true, true, true
	stopArray[','], stopArray[']'], stopArray[eoi] = true, true, true
	stopX[eoi] = true
	stopAll[','], stopAll[':'], stopAll[']'], stopAll['}'], stopAll[eoi] = true, true, true, true, true
}

func (s *stopSet) has(c uint16) bool {
	return c < maxStop && s[c]
}

// Parse parses text the way json-smart's JSONParser in permissive mode does: single quotes and unquoted strings
// are accepted, NaN becomes a float32 NaN, integers become int32, int64 or *big.Int by magnitude, decimals become
// float64 or, beyond 18 characters, BigDecimal, and anything after the first complete value is ignored.
func Parse(text string) (any, error) {
	p := &parser{in: java.ToChars(text), pos: -1}
	p.read()
	return p.readMain(&stopX, true)
}

type parser struct {
	in    java.Chars
	pos   int
	c     uint16
	depth int
	xs    string
	sb    []uint16
}

func (p *parser) read() {
	p.pos++
	if p.pos >= len(p.in) {
		p.c = eoi
	} else {
		p.c = p.in[p.pos]
	}
}

func (p *parser) readNoEnd() error {
	p.pos++
	if p.pos >= len(p.in) {
		p.c = eoi
		return &ParseError{Position: p.pos - 1, Kind: ErrUnexpectedEOF, Unexpected: "EOF"}
	}
	p.c = p.in[p.pos]
	return nil
}

func charText(c uint16) string {
	return java.Chars{c}.String()
}

func (p *parser) errChar(pos int) error {
	return &ParseError{Position: pos, Kind: ErrUnexpectedChar, Unexpected: charText(p.c)}
}

func (p *parser) errToken(pos int, token string) error {
	return &ParseError{Position: pos, Kind: ErrUnexpectedToken, Unexpected: token}
}

// readMain reads one value; top is true for the document itself (json-smart's readFirst), where the mapper's
// conversion applies and the stop set is EOI only.
func (p *parser) readMain(stop *stopSet, top bool) (any, error) {
	for {
		switch p.c {
		case ' ', '\r', '\n', '\t':
			p.read()
			continue
		case ':', '}', ']':
			return nil, p.errChar(p.pos)
		case '{':
			return p.readObject()
		case '[':
			return p.readArray()
		case '"', '\'':
			if err := p.readString(); err != nil {
				return nil, err
			}
			return p.xs, nil
		case 'n', 'f', 't', 'N':
			first := p.c
			p.readNQString(stop)
			switch {
			case first == 'n' && p.xs == nullToken:
				return nil, nil
			case first == 'f' && p.xs == "false":
				return false, nil
			case first == 't' && p.xs == "true":
				return true, nil
			case first == 'N' && p.xs == "NaN":
				return float32(math.NaN()), nil
			}
			return p.xs, nil
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return p.readNumber(stop)
		default:
			p.readNQString(stop)
			return p.xs, nil
		}
	}
}

// enter counts one more level of nesting and fails beyond json-smart's depth limit.
func (p *parser) enter() error {
	p.depth++
	if p.depth > maxDepth {
		return &ParseError{Position: p.pos, Kind: ErrUnexpectedJSONDepth, Unexpected: charText(p.c)}
	}
	return nil
}

func (p *parser) readArray() (any, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	current := NewArray()
	p.read()
	for {
		switch p.c {
		case ' ', '\r', '\n', '\t':
			p.read()
			continue
		case ']':
			p.depth--
			p.read()
			return current, nil
		case ':', '}':
			return nil, p.errChar(p.pos)
		case ',':
			p.read()
			continue
		case eoi:
			return nil, &ParseError{Position: p.pos - 1, Kind: ErrUnexpectedEOF, Unexpected: "EOF"}
		default:
			v, err := p.readMain(&stopArray, false)
			if err != nil {
				return nil, err
			}
			current.Add(v)
		}
	}
}

func (p *parser) readObject() (any, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	current := NewObject()
	acceptData := true
	for {
		p.read()
		switch p.c {
		case ' ', '\r', '\t', '\n':
			continue
		case ':', ']', '[', '{':
			return nil, p.errChar(p.pos)
		case '}':
			p.depth--
			p.read()
			return current, nil
		case ',':
			acceptData = true
			continue
		}
		if p.c == '"' || p.c == '\'' {
			if err := p.readString(); err != nil {
				return nil, err
			}
		} else {
			p.readNQString(&stopKey)
		}
		key := p.xs
		if !acceptData {
			return nil, p.errToken(p.pos, key)
		}
		p.skipSpace()
		if p.c != ':' {
			if p.c == eoi {
				return nil, &ParseError{Position: p.pos - 1, Kind: ErrUnexpectedEOF, Unexpected: nullToken}
			}
			return nil, p.errChar(p.pos - 1)
		}
		if err := p.readNoEnd(); err != nil {
			return nil, err
		}
		value, err := p.readMain(&stopValue, false)
		if err != nil {
			return nil, err
		}
		current.Put(key, value)
		p.skipSpace()
		switch p.c {
		case '}':
			p.depth--
			p.read()
			return current, nil
		case eoi:
			return nil, &ParseError{Position: p.pos - 1, Kind: ErrUnexpectedEOF, Unexpected: nullToken}
		case ',':
			acceptData = true
		default:
			return nil, p.errToken(p.pos-1, charText(p.c))
		}
	}
}

func (p *parser) skipDigits() {
	for p.c >= '0' && p.c <= '9' {
		p.read()
	}
}

func (p *parser) skipNQString(stop *stopSet) {
	for p.c != eoi && !stop.has(p.c) {
		p.read()
	}
}

func (p *parser) skipSpace() {
	for p.c <= ' ' && p.c != eoi {
		p.read()
	}
}

func (p *parser) readNQString(stop *stopSet) {
	start := p.pos
	p.skipNQString(stop)
	p.extractStringTrim(start, p.pos)
}

func (p *parser) extractString(start, stop int) {
	p.xs = p.in[start:stop].String()
}

func (p *parser) extractStringTrim(start, stop int) {
	for start < stop-1 && java.IsWhitespace(rune(p.in[start])) {
		start++
	}
	for stop-1 > start && java.IsWhitespace(rune(p.in[stop-1])) {
		stop--
	}
	p.extractString(start, stop)
}

// unquoted returns the token as an unquoted string once a number turns out to continue with other characters.
func (p *parser) unquoted(start int, stop *stopSet) (any, error) {
	p.skipNQString(stop)
	p.extractStringTrim(start, p.pos)
	return p.xs, nil
}

func (p *parser) readNumber(stop *stopSet) (any, error) {
	start := p.pos
	p.read()
	p.skipDigits()
	if p.c != '.' && p.c != 'E' && p.c != 'e' {
		p.skipSpace()
		if p.c < maxStop && !stop.has(p.c) && p.c != eoi {
			return p.unquoted(start, stop)
		}
		p.extractStringTrim(start, p.pos)
		return parseInteger(p.xs), nil
	}
	if p.c == '.' {
		p.read()
		p.skipDigits()
	}
	if p.c != 'E' && p.c != 'e' {
		p.skipSpace()
		if p.c < maxStop && !stop.has(p.c) && p.c != eoi {
			return p.unquoted(start, stop)
		}
		p.extractStringTrim(start, p.pos)
		return p.extractFloat()
	}
	p.read()
	if p.c == '+' || p.c == '-' || p.c >= '0' && p.c <= '9' {
		p.read()
		p.skipDigits()
		p.skipSpace()
		if p.c < maxStop && !stop.has(p.c) && p.c != eoi {
			return p.unquoted(start, stop)
		}
		p.extractStringTrim(start, p.pos)
		return p.extractFloat()
	}
	return p.unquoted(start, stop)
}

// parseInteger converts an integer token: int32 when it fits, then int64, then *big.Int. A lone "-" is 0.
func parseInteger(s string) any {
	digits := strings.TrimPrefix(s, "-")
	neg := len(digits) < len(s)
	if len(digits) > 19 {
		v, _ := new(big.Int).SetString(s, 10)
		return v
	}
	if digits == "" {
		return int32(0)
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		b, _ := new(big.Int).SetString(s, 10)
		return b
	}
	if neg && v == 0 {
		return int32(0)
	}
	if v >= -1<<31 && v <= 1<<31-1 {
		return int32(v)
	}
	return v
}

func (p *parser) extractFloat() (any, error) {
	if java.Len(p.xs) > 18 {
		d, err := java.ParseDecimal(p.xs)
		if err != nil {
			return nil, p.errToken(p.pos, p.xs)
		}
		return d, nil
	}
	v, err := java.ParseDouble(p.xs)
	if err != nil {
		return nil, p.errToken(p.pos, p.xs)
	}
	return v, nil
}

func (p *parser) readString() error {
	tmp := -1
	for i := p.pos + 1; i < len(p.in); i++ {
		if p.in[i] == p.c {
			tmp = i
			break
		}
	}
	if tmp == -1 {
		return &ParseError{Position: len(p.in), Kind: ErrUnexpectedEOF, Unexpected: nullToken}
	}
	raw := p.in[p.pos+1 : tmp]
	if !containsUnit(raw, '\\') {
		// Permissive mode ignores control characters, so a string without escapes is taken as it is.
		p.xs = raw.String()
		p.pos = tmp
		p.read()
		return nil
	}
	p.sb = p.sb[:0]
	return p.readEscapedString()
}

func containsUnit(s java.Chars, c uint16) bool {
	for _, u := range s {
		if u == c {
			return true
		}
	}
	return false
}

func (p *parser) readEscapedString() error {
	sep := p.c
	for {
		p.read()
		switch {
		case p.c == eoi:
			return &ParseError{Position: p.pos - 1, Kind: ErrUnexpectedEOF, Unexpected: nullToken}
		case p.c == '"' || p.c == '\'':
			if p.c == sep {
				p.read()
				p.xs = java.Chars(p.sb).String()
				return nil
			}
			p.sb = append(p.sb, p.c)
		case p.c == '\\':
			p.read()
			switch p.c {
			case 't':
				p.sb = append(p.sb, '\t')
			case 'n':
				p.sb = append(p.sb, '\n')
			case 'r':
				p.sb = append(p.sb, '\r')
			case 'f':
				p.sb = append(p.sb, '\f')
			case 'b':
				p.sb = append(p.sb, '\b')
			case '\\', '/', '\'', '"':
				p.sb = append(p.sb, p.c)
			case 'u', 'x':
				n := 4
				if p.c == 'x' {
					n = 2
				}
				u, err := p.readUnicode(n)
				if err != nil {
					return err
				}
				p.sb = append(p.sb, u)
			}
		case p.c <= 31 || p.c == 127:
			// Permissive mode drops control characters from strings that hold escapes.
		default:
			p.sb = append(p.sb, p.c)
		}
	}
}

func (p *parser) readUnicode(n int) (uint16, error) {
	var value uint16
	for i := 0; i < n; i++ {
		value *= 16
		p.read()
		switch {
		case p.c >= '0' && p.c <= '9':
			value += p.c - '0'
		case p.c >= 'A' && p.c <= 'F':
			value += p.c - 'A' + 10
		case p.c >= 'a' && p.c <= 'f':
			value += p.c - 'a' + 10
		case p.c == eoi:
			return 0, &ParseError{Position: p.pos, Kind: ErrUnexpectedEOF, Unexpected: "EOF"}
		default:
			return 0, &ParseError{Position: p.pos, Kind: ErrUnexpectedUnicode, Unexpected: charText(p.c)}
		}
	}
	return value, nil
}
