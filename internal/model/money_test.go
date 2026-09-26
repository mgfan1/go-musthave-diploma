package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoneyMarshalJSON(t *testing.T) {
	cases := []struct {
		name  string
		value Money
		want  string
	}{
		{"копейки", 729.98, "729.98"},
		{"десятые", 500.5, "500.5"},
		{"целое", 42, "42"},
		{"ноль", 0, "0"},
		{"круглые сотни", 100, "100"},
		{"хвост двоичной дроби", 0.1 + 0.2, "0.3"},
		{"хвост после копеек", 729.9800000000001, "729.98"},
		{"верхняя граница numeric(12,2)", 9999999999.99, "9999999999.99"},
		{"одна копейка", 0.01, "0.01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := json.Marshal(c.value)
			require.NoError(t, err)
			assert.Equal(t, c.want, string(data))
		})
	}
}

func TestMoneyUnmarshalJSON(t *testing.T) {
	var m Money
	require.NoError(t, json.Unmarshal([]byte("729.98"), &m))
	assert.Equal(t, Money(729.98), m)
}
