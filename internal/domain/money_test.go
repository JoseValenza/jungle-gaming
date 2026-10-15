
package domain

import "testing"

func TestParseMoney(t *testing.T) {
	tests := []struct {
		input string
		want  Money
		valid bool
	}{
		{"100.50", 10050, true},
		{"20", 2000, true},
		{"0.99", 99, true},
		{"10.999", 0, false},
		{"-10.00", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, err := ParseMoney(tt.input)

		if tt.valid && err != nil {
			t.Errorf("ParseMoney(%q): erro inesperado: %v", tt.input, err)
		}

		if !tt.valid && err == nil {
			t.Errorf("ParseMoney(%q): esperava erro", tt.input)
		}

		if tt.valid && got != tt.want {
			t.Errorf("ParseMoney(%q) = %d; esperado %d", tt.input, got, tt.want)
		}
	}
}

func TestMoneyString(t *testing.T) {
	got := Money(10050).String()

	if got != "100.50" {
		t.Errorf("String() = %q; esperado %q", got, "100.50")
	}
}

func TestMoneyAdd(t *testing.T) {
	got, err := Money(1000).Add(Money(500))

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if got != Money(1500) {
		t.Errorf("Add() = %d; esperado 1500", got)
	}
}

func TestMoneySub(t *testing.T) {
	got, err := Money(1000).Sub(Money(300))

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if got != Money(700) {
		t.Errorf("Sub() = %d; esperado 700", got)
	}
}
