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

// Package javaregex matches strings with java.util.regex patterns, the dialect of JsonPath's =~ operator. A
// pattern is translated into an equivalent Go regular expression: Java's flags, ASCII-only case folding, line
// terminators, predefined and POSIX classes, nested classes and quoting are rewritten into RE2 syntax. Constructs
// RE2 cannot express (back-references, look-around, atomic groups, possessive quantifiers, class intersections)
// are reported as unsupported.
package javaregex

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Flags of java.util.regex.Pattern.
const (
	UnixLines             = 0x01
	CaseInsensitive       = 0x02
	Comments              = 0x04
	Multiline             = 0x08
	DotAll                = 0x20
	UnicodeCase           = 0x40
	UnicodeCharacterClass = 0x100
)

var flagLetters = []struct {
	letter rune
	flag   int
}{
	{'d', UnixLines}, {'i', CaseInsensitive}, {'x', Comments}, {'m', Multiline}, {'s', DotAll},
	{'u', UnicodeCase}, {'U', UnicodeCharacterClass},
}

// FlagOf returns the flag a JsonPath pattern flag letter stands for, or 0.
func FlagOf(letter rune) int {
	for _, f := range flagLetters {
		if f.letter == letter {
			return f.flag
		}
	}
	return 0
}

// ParseFlags combines the flags named by the letters of s; unknown letters are ignored.
func ParseFlags(s string) int {
	flags := 0
	for _, r := range s {
		flags |= FlagOf(r)
	}
	return flags
}

// Regexp is a compiled Java pattern.
type Regexp struct {
	re *regexp.Regexp
}

// SyntaxError reports a pattern Java rejects or this package cannot translate.
type SyntaxError struct {
	Pattern string
	Reason  string
}

// Error describes why the pattern was rejected.
func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%s near pattern %q", e.Reason, e.Pattern)
}

// Compile translates a Java pattern with the given flags.
func Compile(pattern string, flags int) (*Regexp, error) {
	if flags&UnicodeCharacterClass != 0 {
		flags |= UnicodeCase
	}
	t := &translator{src: []rune(pattern), pattern: pattern}
	body, err := t.sequence(flags, true)
	if err != nil {
		return nil, err
	}
	if t.pos < len(t.src) {
		return nil, t.fail("Unmatched closing ')'")
	}
	re, err := regexp.Compile(`\A(?:` + body + `)\z`)
	if err != nil {
		return nil, &SyntaxError{Pattern: pattern, Reason: err.Error()}
	}
	return &Regexp{re: re}, nil
}

// Matches reports whether the whole of s matches, as Matcher.matches.
func (r *Regexp) Matches(s string) bool {
	return r.re.MatchString(s)
}

type translator struct {
	src     []rune
	pos     int
	pattern string
}

func (t *translator) fail(reason string) error {
	return &SyntaxError{Pattern: t.pattern, Reason: reason}
}

func (t *translator) more() bool {
	return t.pos < len(t.src)
}

func (t *translator) peek() rune {
	return t.src[t.pos]
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r'
}

// skipComments skips whitespace and #-comments in COMMENTS mode.
func (t *translator) skipComments(flags int) {
	if flags&Comments == 0 {
		return
	}
	for t.more() {
		switch c := t.peek(); {
		case isSpace(c):
			t.pos++
		case c == '#':
			for t.more() && !isLineTerminator(t.peek(), flags) {
				t.pos++
			}
		default:
			return
		}
	}
}

func isLineTerminator(r rune, flags int) bool {
	if flags&UnixLines != 0 {
		return r == '\n'
	}
	return r == '\n' || r == '\r' || r == 0x85 || r == 0x2028 || r == 0x2029
}

// sequence translates alternatives up to an unmatched ')' or the end.
func (t *translator) sequence(flags int, top bool) (string, error) {
	var out strings.Builder
	for {
		t.skipComments(flags)
		if !t.more() {
			return out.String(), nil
		}
		c := t.peek()
		switch c {
		case ')':
			if top {
				return "", t.fail("Unmatched closing ')'")
			}
			return out.String(), nil
		case '|':
			t.pos++
			out.WriteByte('|')
			continue
		}
		atom, newFlags, isFlagGroup, err := t.atom(flags)
		if err != nil {
			return "", err
		}
		if isFlagGroup {
			flags = newFlags
			continue
		}
		q, err := t.quantifier(flags, atom != "")
		if err != nil {
			return "", err
		}
		if atom == "" && q != "" {
			return "", t.fail("Dangling meta character")
		}
		out.WriteString(atom)
		out.WriteString(q)
	}
}

func (t *translator) quantifier(flags int, hasAtom bool) (string, error) {
	t.skipComments(flags)
	if !t.more() {
		return "", nil
	}
	var q string
	switch c := t.peek(); c {
	case '*', '+', '?':
		t.pos++
		q = string(c)
	case '{':
		start := t.pos
		t.pos++
		end := t.pos
		for end < len(t.src) && t.src[end] != '}' {
			end++
		}
		if end >= len(t.src) {
			return "", t.fail("Unclosed counted closure")
		}
		body := string(t.src[t.pos:end])
		lo, hi, hasComma := strings.Cut(body, ",")
		if !allDigits(lo) || lo == "" || hasComma && hi != "" && !allDigits(hi) {
			t.pos = start
			return "", t.fail("Illegal repetition")
		}
		if hasComma && hi != "" {
			a, _ := strconv.Atoi(lo)
			b, _ := strconv.Atoi(hi)
			if b < a {
				return "", t.fail("Illegal repetition range")
			}
		}
		t.pos = end + 1
		q = "{" + body + "}"
	default:
		return "", nil
	}
	if !hasAtom {
		return q, nil
	}
	if t.more() {
		switch t.peek() {
		case '?':
			t.pos++
			q += "?"
		case '+':
			return "", t.fail("possessive quantifiers are not supported")
		}
	}
	return q, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// atom translates one atom. A flag group such as (?i) returns the flags it sets instead.
func (t *translator) atom(flags int) (string, int, bool, error) {
	c := t.peek()
	t.pos++
	switch c {
	case '(':
		return t.group(flags)
	case '[':
		set, err := t.class(flags)
		if err != nil {
			return "", 0, false, err
		}
		return set.render(), 0, false, nil
	case '.':
		switch {
		case flags&DotAll != 0:
			return `(?s:.)`, 0, false, nil
		case flags&UnixLines != 0:
			return `[^\n]`, 0, false, nil
		}
		return `[^\n\r\x{85}\x{2028}\x{2029}]`, 0, false, nil
	case '^':
		if flags&Multiline != 0 {
			return `(?m:^)`, 0, false, nil
		}
		return `\A`, 0, false, nil
	case '$':
		if flags&Multiline != 0 {
			return `(?m:$)`, 0, false, nil
		}
		return `\z`, 0, false, nil
	case '\\':
		s, err := t.escape(flags)
		return s, 0, false, err
	case '*', '+', '?', '{':
		t.pos--
		return "", 0, false, nil
	}
	return literal(c, flags), 0, false, nil
}

func (t *translator) group(flags int) (string, int, bool, error) {
	open := "("
	inner := flags
	if t.more() && t.peek() == '?' {
		t.pos++
		if !t.more() {
			return "", 0, false, t.fail("Unknown group type")
		}
		switch c := t.peek(); {
		case c == ':':
			t.pos++
			open = "(?:"
		case c == '<':
			t.pos++
			if t.more() && (t.peek() == '=' || t.peek() == '!') {
				return "", 0, false, t.fail("look-behind is not supported")
			}
			start := t.pos
			for t.more() && (unicode.IsLetter(t.peek()) && t.peek() < 128 || unicode.IsDigit(t.peek()) && t.peek() < 128) {
				t.pos++
			}
			if !t.more() || t.peek() != '>' || t.pos == start || !unicode.IsLetter(t.src[start]) {
				return "", 0, false, t.fail("named capturing group is missing trailing '>'")
			}
			name := string(t.src[start:t.pos])
			t.pos++
			open = "(?P<" + name + ">"
		case c == '=' || c == '!' || c == '>':
			return "", 0, false, t.fail("look-ahead and atomic groups are not supported")
		default:
			on := true
			for t.more() {
				l := t.peek()
				if l == '-' {
					on = false
					t.pos++
					continue
				}
				f := FlagOf(l)
				if f == 0 && l != 'c' {
					break
				}
				t.pos++
				if on {
					inner |= f
				} else {
					inner &^= f
				}
			}
			if inner&UnicodeCharacterClass != 0 {
				inner |= UnicodeCase
			}
			if !t.more() {
				return "", 0, false, t.fail("Unknown inline modifier")
			}
			switch t.peek() {
			case ')':
				t.pos++
				return "", inner, true, nil
			case ':':
				t.pos++
				open = "(?:"
			default:
				return "", 0, false, t.fail("Unknown inline modifier")
			}
		}
	}
	body, err := t.sequence(inner, false)
	if err != nil {
		return "", 0, false, err
	}
	if !t.more() || t.peek() != ')' {
		return "", 0, false, t.fail("Unclosed group")
	}
	t.pos++
	return open + body + ")", 0, false, nil
}

// literal renders one character, folding case the way the flags ask for.
func literal(c rune, flags int) string {
	if flags&CaseInsensitive != 0 {
		if flags&UnicodeCase != 0 {
			// A single character matches every character with the same lower case of its upper case.
			target := unicode.ToLower(unicode.ToUpper(c))
			set := setOf(c, c)
			for _, r := range casedRunes() {
				if unicode.ToLower(unicode.ToUpper(r)) == target {
					set.add(r, r)
				}
			}
			if len(set.ranges) > 1 || set.ranges[0][0] != set.ranges[0][1] {
				return set.render()
			}
		} else if c < 128 && unicode.IsLetter(c) {
			return "[" + string(unicode.ToLower(c)) + string(unicode.ToUpper(c)) + "]"
		}
	}
	return quoteRune(c)
}

func quoteRune(c rune) string {
	switch {
	case c == '\n':
		return `\n`
	case c < 0x20 || c == 0x7f || c >= 0x80 && !unicode.IsPrint(c):
		return fmt.Sprintf(`\x{%x}`, c)
	}
	return regexp.QuoteMeta(string(c))
}

// escape translates the escape sequence after a backslash outside a class.
func (t *translator) escape(flags int) (string, error) {
	if !t.more() {
		return "", t.fail("Unexpected internal error")
	}
	c := t.peek()
	switch c {
	case 'Q':
		t.pos++
		var out strings.Builder
		for t.more() {
			if t.peek() == '\\' && t.pos+1 < len(t.src) && t.src[t.pos+1] == 'E' {
				t.pos += 2
				return out.String(), nil
			}
			out.WriteString(literal(t.peek(), flags))
			t.pos++
		}
		return out.String(), nil
	case 'b', 'B':
		t.pos++
		return `\` + string(c), nil
	case 'A':
		t.pos++
		return `\A`, nil
	case 'z':
		t.pos++
		return `\z`, nil
	case 'Z':
		t.pos++
		return `(?:\n?\z)`, nil
	case 'G':
		t.pos++
		return `\A`, nil
	case 'R':
		t.pos++
		return `(?:\r\n|[\n\x0B\f\r\x{85}\x{2028}\x{2029}])`, nil
	}
	if c >= '1' && c <= '9' || c == 'k' {
		return "", t.fail("back-references are not supported")
	}
	set, single, err := t.classEscape(flags)
	if err != nil {
		return "", err
	}
	if set != nil {
		return set.render(), nil
	}
	return literal(single, flags), nil
}

// classEscape reads an escape that may appear inside or outside a class: either a predefined class (set) or one
// character.
func (t *translator) classEscape(flags int) (*runeSet, rune, error) {
	c := t.peek()
	t.pos++
	switch c {
	case 'd', 'D', 'w', 'W', 's', 'S', 'h', 'H', 'v', 'V':
		set := predefined(c, flags)
		return set, 0, nil
	case 'p', 'P':
		set, err := t.property(flags)
		if err != nil {
			return nil, 0, err
		}
		if c == 'P' {
			set = set.complement()
		}
		return set, 0, nil
	case 't':
		return nil, '\t', nil
	case 'n':
		return nil, '\n', nil
	case 'r':
		return nil, '\r', nil
	case 'f':
		return nil, '\f', nil
	case 'a':
		return nil, 7, nil
	case 'e':
		return nil, 27, nil
	case '0':
		v := 0
		n := 0
		for t.more() && n < 3 && t.peek() >= '0' && t.peek() <= '7' {
			next := v*8 + int(t.peek()-'0')
			if next > 0377 {
				break
			}
			v = next
			t.pos++
			n++
		}
		if n == 0 {
			return nil, 0, t.fail("Illegal octal escape sequence")
		}
		return nil, rune(v), nil
	case 'x':
		if t.more() && t.peek() == '{' {
			end := t.pos + 1
			for end < len(t.src) && t.src[end] != '}' {
				end++
			}
			if end >= len(t.src) {
				return nil, 0, t.fail("Unclosed hexadecimal escape sequence")
			}
			v, err := strconv.ParseUint(string(t.src[t.pos+1:end]), 16, 32)
			if err != nil || v > unicode.MaxRune {
				return nil, 0, t.fail("Illegal hexadecimal escape sequence")
			}
			t.pos = end + 1
			return nil, rune(v), nil
		}
		return t.hex(2)
	case 'u':
		return t.hex(4)
	case 'c':
		if !t.more() {
			return nil, 0, t.fail("Illegal control escape sequence")
		}
		r := t.peek() ^ 64
		t.pos++
		return nil, r, nil
	}
	if c < 128 && (unicode.IsLetter(c) || unicode.IsDigit(c)) {
		return nil, 0, t.fail("Illegal/unsupported escape sequence")
	}
	return nil, c, nil
}

func (t *translator) hex(n int) (*runeSet, rune, error) {
	if t.pos+n > len(t.src) {
		return nil, 0, t.fail("Illegal hexadecimal escape sequence")
	}
	v, err := strconv.ParseUint(string(t.src[t.pos:t.pos+n]), 16, 32)
	if err != nil {
		return nil, 0, t.fail("Illegal hexadecimal escape sequence")
	}
	t.pos += n
	return nil, rune(v), nil
}

// class translates a bracket expression after its '['.
func (t *translator) class(flags int) (*runeSet, error) {
	negate := false
	if t.more() && t.peek() == '^' {
		negate = true
		t.pos++
	}
	set, err := t.classBody(flags, true)
	if err != nil {
		return nil, err
	}
	if negate {
		return set.complement(), nil
	}
	return set, nil
}

// classBody reads class members up to and including the closing ']'. Literal members fold case as the flags ask;
// property members already carry Java's case-insensitive meaning. An && intersects what precedes with the rest.
func (t *translator) classBody(flags int, first bool) (*runeSet, error) {
	literals, props := &runeSet{}, &runeSet{}
	result := func() *runeSet {
		out := literals.fold(flags)
		out.addSet(props)
		return out
	}
	for {
		t.skipComments(flags)
		if !t.more() {
			return nil, t.fail("Unclosed character class")
		}
		c := t.peek()
		switch {
		case c == ']' && !first:
			t.pos++
			return result(), nil
		case c == '[':
			t.pos++
			nested, err := t.class(flags)
			if err != nil {
				return nil, err
			}
			props.addSet(nested)
			first = false
			continue
		case c == '&' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '&':
			t.pos += 2
			rest, err := t.classBody(flags, false)
			if err != nil {
				return nil, err
			}
			return result().intersect(rest), nil
		}
		first = false
		set, single, err := t.classAtom(flags)
		if err != nil {
			return nil, err
		}
		if single == nil {
			props.addSet(set)
			continue
		}
		if t.pos+1 < len(t.src) && t.peek() == '-' && t.src[t.pos+1] != ']' && t.src[t.pos+1] != '[' {
			t.pos++
			_, hi, err := t.classAtom(flags)
			if err != nil {
				return nil, err
			}
			if hi == nil || *hi < *single {
				return nil, t.fail("Illegal character range")
			}
			literals.add(*single, *hi)
			continue
		}
		literals.add(*single, *single)
	}
}

func (s *runeSet) intersect(o *runeSet) *runeSet {
	out := &runeSet{}
	for _, a := range s.ranges {
		for _, b := range o.ranges {
			lo, hi := max(a[0], b[0]), min(a[1], b[1])
			if lo <= hi {
				out.ranges = append(out.ranges, [2]rune{lo, hi})
			}
		}
	}
	out.normalize()
	return out
}

func (t *translator) classAtom(flags int) (*runeSet, *rune, error) {
	c := t.peek()
	t.pos++
	if c != '\\' {
		return nil, &c, nil
	}
	if !t.more() {
		return nil, nil, t.fail("Unclosed character class")
	}
	if t.peek() == 'Q' {
		return nil, nil, t.fail("quoting inside classes is not supported")
	}
	set, r, err := t.classEscape(flags)
	if err != nil {
		return nil, nil, err
	}
	if set != nil {
		return set, nil, nil
	}
	return nil, &r, nil
}

// property reads the name of a \p{...} or \pX class.
func (t *translator) property(flags int) (*runeSet, error) {
	if !t.more() {
		return nil, t.fail("Illegal character property")
	}
	var name string
	if t.peek() == '{' {
		end := t.pos + 1
		for end < len(t.src) && t.src[end] != '}' {
			end++
		}
		if end >= len(t.src) {
			return nil, t.fail("Unclosed character family")
		}
		name = string(t.src[t.pos+1 : end])
		t.pos = end + 1
	} else {
		name = string(t.peek())
		t.pos++
	}
	set := propertySet(name, flags)
	if set == nil {
		return nil, t.fail("Unknown character property name {" + name + "}")
	}
	return set, nil
}

// runeSet is a set of code points as sorted, non-overlapping ranges.
type runeSet struct {
	ranges [][2]rune
}

func (s *runeSet) add(lo, hi rune) {
	s.ranges = append(s.ranges, [2]rune{lo, hi})
	s.normalize()
}

func (s *runeSet) addSet(o *runeSet) {
	s.ranges = append(s.ranges, o.ranges...)
	s.normalize()
}

func (s *runeSet) normalize() {
	sort.Slice(s.ranges, func(i, j int) bool { return s.ranges[i][0] < s.ranges[j][0] })
	var out [][2]rune
	for _, r := range s.ranges {
		if n := len(out); n > 0 && r[0] <= out[n-1][1]+1 {
			if r[1] > out[n-1][1] {
				out[n-1][1] = r[1]
			}
			continue
		}
		out = append(out, r)
	}
	s.ranges = out
}

func (s *runeSet) complement() *runeSet {
	out := &runeSet{}
	next := rune(0)
	for _, r := range s.ranges {
		if r[0] > next {
			out.ranges = append(out.ranges, [2]rune{next, r[0] - 1})
		}
		next = r[1] + 1
	}
	if next <= unicode.MaxRune {
		out.ranges = append(out.ranges, [2]rune{next, unicode.MaxRune})
	}
	return out
}

// fold adds the case variants a case-insensitive class matches. Without UNICODE_CASE only ASCII letters fold. With
// it, Java matches a character when the character, its upper case, its lower case or the lower case of its upper
// case is in the class.
func (s *runeSet) fold(flags int) *runeSet {
	if flags&CaseInsensitive == 0 {
		return s
	}
	out := &runeSet{ranges: append([][2]rune(nil), s.ranges...)}
	if flags&UnicodeCase == 0 {
		for _, r := range s.ranges {
			for c := max(r[0], 'A'); c <= min(r[1], 'z'); c++ {
				if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
					out.ranges = append(out.ranges, [2]rune{c ^ 0x20, c ^ 0x20})
				}
			}
		}
		out.normalize()
		return out
	}
	for _, c := range casedRunes() {
		if s.contains(c) {
			continue
		}
		up, low := unicode.ToUpper(c), unicode.ToLower(c)
		if s.contains(up) || s.contains(low) || s.contains(unicode.ToLower(up)) {
			out.ranges = append(out.ranges, [2]rune{c, c})
		}
	}
	out.normalize()
	return out
}

// casedRunes lists the characters that have an upper or lower case mapping.
var casedRunes = sync.OnceValue(func() []rune {
	var cased []rune
	for c := rune(0); c <= unicode.MaxRune; c++ {
		if unicode.ToUpper(c) != c || unicode.ToLower(c) != c {
			cased = append(cased, c)
		}
	}
	return cased
})

func (s *runeSet) contains(c rune) bool {
	i := sort.Search(len(s.ranges), func(i int) bool { return s.ranges[i][1] >= c })
	return i < len(s.ranges) && s.ranges[i][0] <= c
}

func (s *runeSet) render() string {
	if len(s.ranges) == 0 {
		return `[^\x00-\x{10FFFF}]`
	}
	var b strings.Builder
	b.WriteByte('[')
	for _, r := range s.ranges {
		b.WriteString(classChar(r[0]))
		if r[1] != r[0] {
			b.WriteByte('-')
			b.WriteString(classChar(r[1]))
		}
	}
	b.WriteByte(']')
	return b.String()
}

func classChar(r rune) string {
	if r < 0x20 || r > 0x7e {
		return fmt.Sprintf(`\x{%x}`, r)
	}
	if strings.ContainsRune(`\]^-[`, r) {
		return `\` + string(r)
	}
	return string(r)
}

func setOf(ranges ...rune) *runeSet {
	s := &runeSet{}
	for i := 0; i+1 < len(ranges); i += 2 {
		s.ranges = append(s.ranges, [2]rune{ranges[i], ranges[i+1]})
	}
	s.normalize()
	return s
}

func tableSet(tables ...*unicode.RangeTable) *runeSet {
	s := &runeSet{}
	for _, t := range tables {
		for _, r := range t.R16 {
			for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
				if r.Stride == 1 {
					s.ranges = append(s.ranges, [2]rune{rune(r.Lo), rune(r.Hi)})
					break
				}
				s.ranges = append(s.ranges, [2]rune{c, c})
			}
		}
		for _, r := range t.R32 {
			for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
				if r.Stride == 1 {
					s.ranges = append(s.ranges, [2]rune{rune(r.Lo), rune(r.Hi)})
					break
				}
				s.ranges = append(s.ranges, [2]rune{c, c})
			}
		}
	}
	s.normalize()
	return s
}

func predefined(c rune, flags int) *runeSet {
	unicodeClasses := flags&UnicodeCharacterClass != 0
	var set *runeSet
	switch unicode.ToLower(c) {
	case 'd':
		set = setOf('0', '9')
		if unicodeClasses {
			set = tableSet(unicode.Nd)
		}
	case 'w':
		set = setOf('a', 'z', 'A', 'Z', '0', '9', '_', '_')
		if unicodeClasses {
			set = tableSet(unicode.L, unicode.Mn, unicode.Nd, unicode.Pc, unicode.Other_Alphabetic, unicode.Me,
				unicode.Mc)
			set.add(0x200c, 0x200d)
		}
	case 's':
		set = setOf(' ', ' ', '\t', '\r')
		if unicodeClasses {
			set = tableSet(unicode.White_Space)
		}
	case 'h':
		set = setOf(' ', ' ', '\t', '\t', 0xa0, 0xa0, 0x1680, 0x1680, 0x180e, 0x180e, 0x2000, 0x200a, 0x202f, 0x202f,
			0x205f, 0x205f, 0x3000, 0x3000)
	case 'v':
		set = setOf('\n', '\r', 0x85, 0x85, 0x2028, 0x2029)
	}
	if unicode.IsUpper(c) {
		return set.complement()
	}
	return set
}

var posixClasses = map[string]*runeSet{
	"Lower":  setOf('a', 'z'),
	"Upper":  setOf('A', 'Z'),
	"ASCII":  setOf(0, 0x7f),
	"Alpha":  setOf('a', 'z', 'A', 'Z'),
	"Digit":  setOf('0', '9'),
	"Alnum":  setOf('a', 'z', 'A', 'Z', '0', '9'),
	"Punct":  setOf('!', '/', ':', '@', '[', '`', '{', '~'),
	"Graph":  setOf('!', '~'),
	"Print":  setOf(' ', '~'),
	"Blank":  setOf(' ', ' ', '\t', '\t'),
	"Cntrl":  setOf(0, 0x1f, 0x7f, 0x7f),
	"XDigit": setOf('0', '9', 'a', 'f', 'A', 'F'),
	"Space":  setOf(' ', ' ', '\t', '\r'),
}

func casedLetters() *runeSet {
	return tableSet(unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// propertySet resolves \p{name}. Under CASE_INSENSITIVE, Java widens the case categories (Lu, Ll, Lt, the
// javaXxxCase classes and Lower/Upper) to every cased letter, whatever UNICODE_CASE says.
func propertySet(name string, flags int) *runeSet {
	ci := flags&CaseInsensitive != 0
	unicodeClasses := flags&UnicodeCharacterClass != 0
	if set, ok := posixClasses[name]; ok {
		if !unicodeClasses {
			if ci && (name == "Lower" || name == "Upper") {
				return setOf('a', 'z', 'A', 'Z')
			}
			return set
		}
		switch name {
		case "Lower", "Upper":
			if ci {
				return casedLetters()
			}
			if name == "Lower" {
				return tableSet(unicode.Ll, unicode.Other_Lowercase)
			}
			return tableSet(unicode.Lu, unicode.Other_Uppercase)
		case "Alpha":
			return tableSet(unicode.L, unicode.Nl, unicode.Other_Alphabetic)
		case "Digit":
			return tableSet(unicode.Nd)
		case "Alnum":
			return tableSet(unicode.L, unicode.Nl, unicode.Other_Alphabetic, unicode.Nd)
		case "Punct":
			return tableSet(unicode.P)
		case "Space":
			return tableSet(unicode.White_Space)
		case "XDigit":
			return tableSet(unicode.Nd, unicode.Hex_Digit)
		case "Cntrl":
			return tableSet(unicode.Cc)
		case "Blank":
			s := tableSet(unicode.Zs)
			s.add('\t', '\t')
			return s
		case "ASCII":
			return set
		}
		return nil
	}
	switch name {
	case "javaLowerCase", "javaUpperCase", "javaTitleCase":
		if ci {
			return casedLetters()
		}
		switch name {
		case "javaLowerCase":
			return tableSet(unicode.Ll, unicode.Other_Lowercase)
		case "javaUpperCase":
			return tableSet(unicode.Lu, unicode.Other_Uppercase)
		}
		return tableSet(unicode.Lt)
	case "javaWhitespace":
		s := tableSet(unicode.Zs, unicode.Zl, unicode.Zp)
		s = s.minus(setOf(0xa0, 0xa0, 0x2007, 0x2007, 0x202f, 0x202f))
		s.addSet(setOf('\t', '\r', 0x1c, 0x1f))
		return s
	case "javaDigit":
		return tableSet(unicode.Nd)
	case "javaLetter":
		return tableSet(unicode.L)
	case "javaLetterOrDigit":
		return tableSet(unicode.L, unicode.Nd)
	}
	key := name
	for _, prefix := range []string{"Is", "general_category=", "gc="} {
		key = strings.TrimPrefix(key, prefix)
	}
	if ci && (key == "Lu" || key == "Ll" || key == "Lt") {
		return tableSet(unicode.Lu, unicode.Ll, unicode.Lt)
	}
	if t, ok := unicode.Categories[key]; ok {
		return tableSet(t)
	}
	if strings.HasPrefix(name, "In") {
		if t, ok := unicode.Scripts[strings.TrimPrefix(name, "In")]; ok {
			return tableSet(t)
		}
	}
	if t, ok := unicode.Scripts[key]; ok && strings.HasPrefix(name, "Is") {
		return tableSet(t)
	}
	switch key {
	case "Alphabetic":
		return tableSet(unicode.L, unicode.Nl, unicode.Other_Alphabetic)
	case "Letter":
		return tableSet(unicode.L)
	case "Digit":
		return tableSet(unicode.Nd)
	case "Lowercase":
		if ci {
			return casedLetters()
		}
		return tableSet(unicode.Ll, unicode.Other_Lowercase)
	case "Uppercase":
		if ci {
			return casedLetters()
		}
		return tableSet(unicode.Lu, unicode.Other_Uppercase)
	case "White_Space", "WhiteSpace":
		return tableSet(unicode.White_Space)
	case "Punctuation":
		return tableSet(unicode.P)
	}
	return nil
}

func (s *runeSet) minus(o *runeSet) *runeSet {
	keep := o.complement()
	out := &runeSet{}
	for _, a := range s.ranges {
		for _, b := range keep.ranges {
			lo, hi := max(a[0], b[0]), min(a[1], b[1])
			if lo <= hi {
				out.ranges = append(out.ranges, [2]rune{lo, hi})
			}
		}
	}
	out.normalize()
	return out
}
