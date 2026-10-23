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

import "github.com/json-path/JsonPath/internal/java"

// charIndex is a cursor over a path or filter expression in UTF-16 code units, the unit of every position JsonPath
// reports. Reading a unit outside the underlying text aborts the compilation with a string index failure: the
// compilers recover it (see recoverIndex) and report it the way JsonPath reports an unexpected exception.
type charIndex struct {
	text     java.Chars
	position int
	end      int
}

// indexFault carries a string index failure from a charIndex read up to the compiler that recovers it.
type indexFault struct {
	err *Error
}

func newCharIndex(s string) *charIndex {
	text := java.ToChars(s)
	return &charIndex{text: text, end: len(text) - 1}
}

// recoverIndex turns an indexFault panic raised by a charIndex read into *err. Other panics are not handled.
func recoverIndex(err *error) {
	if r := recover(); r != nil {
		fault, ok := r.(indexFault)
		if !ok {
			panic(r)
		}
		*err = fault.err
	}
}

func (ci *charIndex) length() int {
	return ci.end + 1
}

func (ci *charIndex) charAt(i int) uint16 {
	if i < 0 || i >= len(ci.text) {
		panic(indexFault{newError(KindStringIndexOutOfBounds, "Index %d out of bounds for length %d", i, len(ci.text))})
	}
	return ci.text[i]
}

func (ci *charIndex) currentChar() uint16 {
	return ci.charAt(ci.position)
}

func (ci *charIndex) currentCharIs(c uint16) bool {
	return ci.charAt(ci.position) == c
}

func (ci *charIndex) lastCharIs(c uint16) bool {
	return ci.charAt(ci.end) == c
}

func (ci *charIndex) nextCharIs(c uint16) bool {
	return ci.inBounds(ci.position+1) && ci.charAt(ci.position+1) == c
}

func (ci *charIndex) incrementPosition(n int) int {
	ci.position += n
	return ci.position
}

func (ci *charIndex) decrementEndPosition(n int) {
	ci.end -= n
}

func (ci *charIndex) setPosition(p int) int {
	ci.position = p
	return p
}

func (ci *charIndex) indexOfMatchingCloseChar(start int, open, close uint16, skipStrings, skipRegex bool) (int, error) {
	if ci.charAt(start) != open {
		return 0, invalidPath("Expected %s but found %s", charString(open), charString(ci.charAt(start)))
	}
	opened := 1
	read := start + 1
	for ci.inBounds(read) {
		if skipStrings {
			q := ci.charAt(read)
			if q == '\'' || q == '"' {
				read = ci.nextIndexOfUnescaped(read, q)
				if read == -1 {
					return 0, invalidPath("Could not find matching close quote for %s when parsing : %s", charString(q), ci.String())
				}
				read++
			}
		}
		if skipRegex && ci.charAt(read) == '/' {
			read = ci.nextIndexOfUnescaped(read, '/')
			if read == -1 {
				return 0, invalidPath("Could not find matching close for / when parsing regex in : %s", ci.String())
			}
			read++
		}
		if ci.charAt(read) == open {
			opened++
		}
		if ci.charAt(read) == close {
			opened--
			if opened == 0 {
				return read, nil
			}
		}
		read++
	}
	return -1, nil
}

func (ci *charIndex) indexOfClosingBracket(start int, skipStrings, skipRegex bool) (int, error) {
	return ci.indexOfMatchingCloseChar(start, '(', ')', skipStrings, skipRegex)
}

func (ci *charIndex) indexOfNextSignificantCharFrom(start int, c uint16) int {
	read := start + 1
	for !ci.isOutOfBounds(read) && ci.charAt(read) == ' ' {
		read++
	}
	if ci.charAt(read) == c {
		return read
	}
	return -1
}

func (ci *charIndex) indexOfNextSignificantChar(c uint16) int {
	return ci.indexOfNextSignificantCharFrom(ci.position, c)
}

func (ci *charIndex) nextIndexOf(start int, c uint16) int {
	for read := start; !ci.isOutOfBounds(read); read++ {
		if ci.charAt(read) == c {
			return read
		}
	}
	return -1
}

func (ci *charIndex) nextIndexOfUnescaped(start int, c uint16) int {
	escaped := false
	for read := start + 1; !ci.isOutOfBounds(read); read++ {
		switch {
		case escaped:
			escaped = false
		case ci.charAt(read) == '\\':
			escaped = true
		case ci.charAt(read) == c:
			return read
		}
	}
	return -1
}

func (ci *charIndex) nextSignificantCharIsFrom(start int, c uint16) bool {
	read := start + 1
	for !ci.isOutOfBounds(read) && ci.charAt(read) == ' ' {
		read++
	}
	return !ci.isOutOfBounds(read) && ci.charAt(read) == c
}

func (ci *charIndex) nextSignificantCharIs(c uint16) bool {
	return ci.nextSignificantCharIsFrom(ci.position, c)
}

func (ci *charIndex) nextSignificantCharFrom(start int) uint16 {
	read := start + 1
	for !ci.isOutOfBounds(read) && ci.charAt(read) == ' ' {
		read++
	}
	if !ci.isOutOfBounds(read) {
		return ci.charAt(read)
	}
	return ' '
}

func (ci *charIndex) nextSignificantChar() uint16 {
	return ci.nextSignificantCharFrom(ci.position)
}

func (ci *charIndex) readSignificantChar(c uint16) error {
	if ci.skipBlanks().currentChar() != c {
		return invalidPath("Expected character: %s", charString(c))
	}
	ci.incrementPosition(1)
	return nil
}

func (ci *charIndex) hasSignificantSubSequence(s string) bool {
	ci.skipBlanks()
	n := len(s)
	if !ci.inBounds(ci.position + n - 1) {
		return false
	}
	if ci.subSequence(ci.position, ci.position+n) != s {
		return false
	}
	ci.incrementPosition(n)
	return true
}

func (ci *charIndex) indexOfPreviousSignificantCharFrom(start int) int {
	read := start - 1
	for !ci.isOutOfBounds(read) && ci.charAt(read) == ' ' {
		read--
	}
	if !ci.isOutOfBounds(read) {
		return read
	}
	return -1
}

func (ci *charIndex) previousSignificantChar() uint16 {
	i := ci.indexOfPreviousSignificantCharFrom(ci.position)
	if i == -1 {
		return ' '
	}
	return ci.charAt(i)
}

func (ci *charIndex) currentIsTail() bool {
	return ci.position >= ci.end
}

func (ci *charIndex) hasMoreCharacters() bool {
	return ci.inBounds(ci.position + 1)
}

func (ci *charIndex) inBounds(i int) bool {
	return i >= 0 && i <= ci.end
}

func (ci *charIndex) inBoundsHere() bool {
	return ci.inBounds(ci.position)
}

func (ci *charIndex) isOutOfBounds(i int) bool {
	return !ci.inBounds(i)
}

// subSequence returns the text from start up to end; a range outside the text aborts like charAt does.
func (ci *charIndex) subSequence(start, end int) string {
	if start < 0 || end > len(ci.text) || start > end {
		panic(indexFault{newError(KindStringIndexOutOfBounds, "Range [%d, %d) out of bounds for length %d", start, end,
			len(ci.text))})
	}
	return ci.text[start:end].String()
}

// String returns the whole underlying text.
func (ci *charIndex) String() string {
	return ci.text.String()
}

// isNumberCharacter reports whether the unit at i can be part of a number literal in a filter.
func (ci *charIndex) isNumberCharacter(i int) bool {
	c := ci.charAt(i)
	return java.IsDigit(rune(c)) || c == '-' || c == '.' || c == 'E' || c == 'e'
}

func (ci *charIndex) skipBlanks() *charIndex {
	for ci.inBoundsHere() && ci.position < ci.end && ci.currentChar() == ' ' {
		ci.incrementPosition(1)
	}
	return ci
}

func (ci *charIndex) skipBlanksAtEnd() {
	for ci.inBoundsHere() && ci.position < ci.end && ci.lastCharIs(' ') {
		ci.decrementEndPosition(1)
	}
}

func (ci *charIndex) trim() *charIndex {
	ci.skipBlanks()
	ci.skipBlanksAtEnd()
	return ci
}

// charString renders one UTF-16 unit as Java prints a char.
func charString(c uint16) string {
	return java.Chars{c}.String()
}
