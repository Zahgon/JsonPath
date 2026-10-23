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
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/json-path/JsonPath/internal/java"
)

// Style selects json-smart's string escaping: StyleWeb (NO_COMPRESS, the default of toString) also escapes '/',
// StyleCompact (LT_COMPRESS) does not.
type Style int

// The two serialisation styles JsonPath uses.
const (
	StyleWeb Style = iota
	StyleCompact
)

// Marshal serialises v as json-smart's JSONValue.toJSONString(v, style).
func Marshal(v any, style Style) string {
	var b strings.Builder
	writeJSON(&b, v, style)
	return b.String()
}

// Quote returns s as a JSON string in the given style.
func Quote(s string, style Style) string {
	var b strings.Builder
	writeString(&b, s, style)
	return b.String()
}

func writeJSON(b *strings.Builder, v any, style Style) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case string:
		writeString(b, t, style)
	case float64:
		if math.IsInf(t, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(java.DoubleToString(t))
		}
	case float32:
		if math.IsInf(float64(t), 0) {
			b.WriteString("null")
		} else {
			b.WriteString(java.FloatToString(t))
		}
	case *Object:
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k, style)
			b.WriteByte(':')
			writeJSON(b, t.values[k], style)
		}
		b.WriteByte('}')
	case *Array:
		writeList(b, t.elems, style)
	case KeySet:
		keys := make([]any, len(t.Object.keys))
		for i, k := range t.Object.keys {
			keys[i] = k
		}
		writeList(b, keys, style)
	default:
		b.WriteString(ToString(v))
	}
}

func writeList(b *strings.Builder, elems []any, style Style) {
	b.WriteByte('[')
	for i, e := range elems {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSON(b, e, style)
	}
	b.WriteByte(']')
}

func writeString(b *strings.Builder, s string, style Style) {
	const hex = "0123456789ABCDEF"
	units := java.ToChars(s)
	b.WriteByte('"')
	for i := 0; i < len(units); i++ {
		c := units[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '/' && style == StyleWeb:
			b.WriteString(`\/`)
		case c <= 0x1f || c >= 0x7f && c <= 0x9f || c >= 0x2000 && c <= 0x20ff:
			b.WriteString(`\u`)
			b.WriteByte(hex[c>>12&0xf])
			b.WriteByte(hex[c>>8&0xf])
			b.WriteByte(hex[c>>4&0xf])
			b.WriteByte(hex[c&0xf])
		case utf16.IsSurrogate(rune(c)) && i+1 < len(units) && utf16.IsSurrogate(rune(units[i+1])):
			b.WriteString(units[i : i+2].String())
			i++
		default:
			b.WriteString(units[i : i+1].String())
		}
	}
	b.WriteByte('"')
}

// ToString renders v as Java's toString of the object json-smart holds: numbers in Java's notation, strings as they
// are, arrays as JSON, objects as {key=value, ...} and key views as [key, ...].
func ToString(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case *big.Int:
		return t.String()
	case float64:
		return java.DoubleToString(t)
	case float32:
		return java.FloatToString(t)
	case BigDecimal:
		return t.String()
	case *Array:
		return Marshal(t, StyleWeb)
	case *Object:
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(ToString(t.values[k]))
		}
		b.WriteByte('}')
		return b.String()
	case KeySet:
		return "[" + strings.Join(t.Object.keys, ", ") + "]"
	}
	return ""
}
