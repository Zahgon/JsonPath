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

// Package java holds the pieces of Java platform behaviour that JsonPath's observable results depend on: UTF-16
// string indexing and ordering, character classes, number parsing and the textual forms of Java numbers.
package java

import (
	"strings"
	"unicode"
	"unicode/utf16"
)

// Chars is a string as a sequence of UTF-16 code units, the unit Java uses for string lengths, indexes and ordering.
type Chars []uint16

// ToChars converts s to UTF-16 code units.
func ToChars(s string) Chars {
	return utf16.Encode([]rune(s))
}

// String converts the code units back to a Go string; unpaired surrogates become U+FFFD.
func (c Chars) String() string {
	return string(utf16.Decode(c))
}

// Len returns the length of s in UTF-16 code units (Java's String.length).
func Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Compare orders a and b by UTF-16 code units, as Java's String.compareTo, and returns the same sign.
func Compare(a, b string) int {
	ca, cb := ToChars(a), ToChars(b)
	n := len(ca)
	if len(cb) < n {
		n = len(cb)
	}
	for i := 0; i < n; i++ {
		if ca[i] != cb[i] {
			return int(ca[i]) - int(cb[i])
		}
	}
	return len(ca) - len(cb)
}

// Trim removes leading and trailing characters up to U+0020, as Java's String.trim.
func Trim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}

// IsWhitespace reports whether c is whitespace by Java's Character.isWhitespace: a Unicode space separator other
// than a no-break space, or one of the ASCII controls TAB, LF, VT, FF, CR and FS to US.
func IsWhitespace(c rune) bool {
	switch c {
	case ' ', ' ', ' ':
		return false
	case '\t', '\n', '\v', '\f', '\r', 0x1c, 0x1d, 0x1e, 0x1f:
		return true
	}
	return unicode.In(c, unicode.Zs, unicode.Zl, unicode.Zp)
}

// IsDigit reports whether c is a decimal digit by Java's Character.isDigit (general category Nd).
func IsDigit(c rune) bool {
	return unicode.Is(unicode.Nd, c)
}

// Digit returns the value of c as a decimal digit, or -1, as Java's Character.digit(c, 10).
func Digit(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 0xff10 && c <= 0xff19:
		return int(c - 0xff10)
	case unicode.Is(unicode.Nd, c):
		// Decimal digits come in contiguous runs of whole sets of ten, each set counting from zero.
		start := c
		for unicode.Is(unicode.Nd, start-1) {
			start--
		}
		return int(c-start) % 10
	}
	return -1
}

// ToUpperRoot upper-cases s as Java's String.toUpperCase(Locale.ROOT).
func ToUpperRoot(s string) string {
	return strings.ToUpperSpecial(unicode.SpecialCase{}, s)
}

// HexUpper returns the upper-case hexadecimal digits of a UTF-16 code unit, as Integer.toHexString(c).toUpperCase().
func HexUpper(c uint16) string {
	const digits = "0123456789ABCDEF"
	if c == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for c > 0 {
		i--
		buf[i] = digits[c&0xf]
		c >>= 4
	}
	return string(buf[i:])
}
