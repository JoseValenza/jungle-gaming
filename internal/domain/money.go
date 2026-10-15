package domain

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var ErrInvalidMoney = errors.New("valor monetário inválido")

//Money - Armazena valores monetários em centavos.
type Money int64

//ParseMoney - Converte uma string decimal em centavos
//Como "100.50" em 10050; "20" em 2000;
func ParseMoney(value string) (Money, error) {
	value = strings.TrimSpace(value)

	if value == "" || strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "+") {
		return 0, ErrInvalidMoney
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalidMoney
	}

	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > math.MaxInt64/100 {
		return 0, ErrInvalidMoney
	}

	var fraction int64
	if len(parts) == 2 {
		decimal := parts[1]
		if len(decimal) < 1 || len(decimal) >2 {
			return 0, ErrInvalidMoney
		}
		if len(decimal) == 1 {
			decimal += "0"
		}
	
		fraction, err = strconv.ParseInt(decimal, 10, 64)
		if err != nil {
			return 0, ErrInvalidMoney
		}
	}

	cents := whole * 100
	if cents > math.MaxInt64-fraction {
		return 0, ErrInvalidMoney
	}

	return Money(cents + fraction), nil
}

//String formata com duas casas decimais.
func (m Money) String() string {
	value := int64(m)
	if value == math.MinInt64 {
		return "-92233720368547758.08"
	}

	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

//Add soma valores com proteção contra overflow.
func (m Money) Add(other Money) (Money, error) {
	a, b := int64(m), int64(other)

	if (b > 0 && a > math.MaxInt64-b) ||
		(b < 0 && a < math.MinInt64-b) {
			return 0, ErrInvalidMoney
	}

	return Money(a + b), nil
}

//Sub - subtrai valores com proteção contra overflow.
func (m Money) Sub(other Money) (Money, error) {
	if other == Money(math.MinInt64) {
		if m >= 0 {
			return 0, ErrInvalidMoney
		}
		return Money(int64(m) - math.MinInt64), nil
	}

	return m.Add(Money(-int64(other)))
}
