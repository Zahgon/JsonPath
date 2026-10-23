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

package java

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Decimal is an immutable arbitrary-precision decimal number, unscaled × 10^-scale, with the semantics of Java's
// BigDecimal where JsonPath relies on them: parsing, ordering, toString and narrowing.
type Decimal struct {
	unscaled *big.Int
	scale    int32
}

// ParseDecimal parses s as new BigDecimal(s): an optional sign, digits of any script with an optional point (at
// least one digit in all) and an optional exponent whose digits are ASCII or any script.
func ParseDecimal(s string) (Decimal, error) {
	fail := &NumberFormatError{Message: "Character array is missing \"exponent\" mark 'e' or 'E'."}
	units := ToChars(s)
	if len(units) == 0 {
		return Decimal{}, &NumberFormatError{Message: "Index 0 out of bounds for length 0"}
	}
	i := 0
	neg := false
	if units[0] == '-' || units[0] == '+' {
		neg = units[0] == '-'
		i++
	}
	var digits strings.Builder
	seenPoint := false
	fracDigits := 0
	nDigits := 0
	for ; i < len(units); i++ {
		r := units[i]
		if d := Digit(rune(r)); d >= 0 {
			digits.WriteByte(byte('0' + d))
			nDigits++
			if seenPoint {
				fracDigits++
			}
			continue
		}
		if r == '.' {
			if seenPoint {
				return Decimal{}, fail
			}
			seenPoint = true
			continue
		}
		if r != 'e' && r != 'E' {
			return Decimal{}, &NumberFormatError{Message: "Character " + units[i:i+1].String() +
				" is neither a decimal digit number, decimal point, nor \"e\" notation exponential mark."}
		}
		break
	}
	if nDigits == 0 {
		return Decimal{}, fail
	}
	var exp int64
	if i < len(units) {
		if units[i] != 'e' && units[i] != 'E' {
			return Decimal{}, fail
		}
		i++
		if i >= len(units) {
			return Decimal{}, fail
		}
		expNeg := false
		if units[i] == '-' || units[i] == '+' {
			expNeg = units[i] == '-'
			i++
		}
		if i >= len(units) {
			return Decimal{}, fail
		}
		for ; i < len(units); i++ {
			d := Digit(rune(units[i]))
			if d < 0 {
				return Decimal{}, fail
			}
			exp = exp*10 + int64(d)
			if exp > math.MaxInt32+1 {
				return Decimal{}, fail
			}
		}
		if expNeg {
			exp = -exp
		}
	}
	scale := int64(fracDigits) - exp
	if scale > math.MaxInt32 || scale < math.MinInt32 {
		return Decimal{}, fail
	}
	u, _ := new(big.Int).SetString(digits.String(), 10)
	if neg {
		u.Neg(u)
	}
	return Decimal{unscaled: u, scale: int32(scale)}, nil
}

// DecimalFromInt64 returns v with scale 0.
func DecimalFromInt64(v int64) Decimal {
	return Decimal{unscaled: big.NewInt(v)}
}

// Cmp compares d and o numerically, as compareTo: -1, 0 or +1.
func (d Decimal) Cmp(o Decimal) int {
	a, b := new(big.Int).Set(d.unscaled), new(big.Int).Set(o.unscaled)
	switch {
	case d.scale < o.scale:
		a.Mul(a, pow10(int64(o.scale)-int64(d.scale)))
	case d.scale > o.scale:
		b.Mul(b, pow10(int64(d.scale)-int64(o.scale)))
	}
	return a.Cmp(b)
}

func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}

// String formats d as BigDecimal.toString: plain notation when the scale is not negative and the adjusted exponent
// is at least -6, scientific notation (d.dddE±n) otherwise.
func (d Decimal) String() string {
	coeff := new(big.Int).Abs(d.unscaled).String()
	sign := ""
	if d.unscaled.Sign() < 0 {
		sign = "-"
	}
	adjusted := -int64(d.scale) + int64(len(coeff)-1)
	if d.scale >= 0 && adjusted >= -6 {
		scale := int(d.scale)
		switch {
		case scale == 0:
			return sign + coeff
		case len(coeff) > scale:
			return sign + coeff[:len(coeff)-scale] + "." + coeff[len(coeff)-scale:]
		default:
			return sign + "0." + strings.Repeat("0", scale-len(coeff)) + coeff
		}
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(coeff[:1])
	if len(coeff) > 1 {
		b.WriteByte('.')
		b.WriteString(coeff[1:])
	}
	b.WriteByte('E')
	if adjusted >= 0 {
		b.WriteByte('+')
	}
	b.WriteString(strconv.FormatInt(adjusted, 10))
	return b.String()
}

// BigInt returns the integer part of d, truncated toward zero, as toBigInteger.
func (d Decimal) BigInt() *big.Int {
	if d.scale <= 0 {
		return new(big.Int).Mul(d.unscaled, pow10(-int64(d.scale)))
	}
	return new(big.Int).Quo(d.unscaled, pow10(int64(d.scale)))
}

// IntValue narrows d to int32 as intValue: the low 32 bits of the truncated integer part.
func (d Decimal) IntValue() int32 {
	return BigIntToInt32(d.BigInt())
}

// Float64 converts d to the nearest float64, as doubleValue.
func (d Decimal) Float64() float64 {
	// ParseFloat rounds correctly and saturates to an infinity or zero out of range, as doubleValue does.
	v, _ := strconv.ParseFloat(d.unscaled.String()+"e"+strconv.FormatInt(-int64(d.scale), 10), 64)
	return v
}

// BigIntToInt32 returns the low 32 bits of v as Java's BigInteger.intValue.
func BigIntToInt32(v *big.Int) int32 {
	return int32(uint32(lowBits(v)))
}

// BigIntToInt64 returns the low 64 bits of v as Java's BigInteger.longValue.
func BigIntToInt64(v *big.Int) int64 {
	return int64(lowBits(v))
}

func lowBits(v *big.Int) uint64 {
	m := new(big.Int).And(v, new(big.Int).SetUint64(math.MaxUint64))
	return m.Uint64()
}

// BigIntToFloat64 converts v to the nearest float64, as BigInteger.doubleValue.
func BigIntToFloat64(v *big.Int) float64 {
	f, _ := new(big.Float).SetInt(v).Float64()
	return f
}
