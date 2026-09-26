package config

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://gophermart:gophermart@localhost:5432/gophermart"

func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"RUN_ADDRESS", "DATABASE_URI", "ACCRUAL_SYSTEM_ADDRESS", "JWT_SECRET"} {
		t.Setenv(name, "")
	}
}

func TestParseDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Parse([]string{"-d", testDSN})
	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Addr)
	assert.Equal(t, testDSN, cfg.DatabaseURI)
	assert.Empty(t, cfg.AccrualAddress)
	assert.Equal(t, defaultJWTSecret, cfg.JWTSecret)
}

func TestParseFlags(t *testing.T) {
	clearEnv(t)

	cfg, err := Parse([]string{"-a", ":9090", "-d", testDSN, "-r", "http://localhost:8081", "-s", "секрет из флага"})
	require.NoError(t, err)
	assert.Equal(t, Config{
		Addr:           ":9090",
		DatabaseURI:    testDSN,
		AccrualAddress: "http://localhost:8081",
		JWTSecret:      "секрет из флага",
	}, cfg)
}

func TestParseEnvBeatsFlag(t *testing.T) {
	clearEnv(t)
	t.Setenv("RUN_ADDRESS", "localhost:8081")
	t.Setenv("DATABASE_URI", testDSN)
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://localhost:8082")
	t.Setenv("JWT_SECRET", "секрет из окружения")

	cfg, err := Parse([]string{"-a", ":9090", "-d", "postgres://flag", "-r", "http://flag:1", "-s", "флаг"})
	require.NoError(t, err)
	assert.Equal(t, Config{
		Addr:           "localhost:8081",
		DatabaseURI:    testDSN,
		AccrualAddress: "http://localhost:8082",
		JWTSecret:      "секрет из окружения",
	}, cfg)
}

func TestParseEmptyEnvKeepsFlag(t *testing.T) {
	clearEnv(t)

	cfg, err := Parse([]string{"-a", ":9090", "-d", testDSN})
	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Addr)
}

func TestParseRequiresDatabase(t *testing.T) {
	clearEnv(t)

	_, err := Parse(nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "DATABASE_URI")
}

func TestParseRequiresSecret(t *testing.T) {
	clearEnv(t)

	_, err := Parse([]string{"-d", testDSN, "-s", ""})
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET")
}

func TestParseFlagErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		help bool
	}{
		{name: "неизвестный флаг", args: []string{"-d", testDSN, "-x"}},
		{name: "флаг без значения", args: []string{"-d"}},
		{name: "справка", args: []string{"-h"}, help: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)

			_, err := Parse(c.args)
			require.Error(t, err)
			assert.Equal(t, c.help, errors.Is(err, flag.ErrHelp))
		})
	}
}

func TestParseRejectsBadAccrualAddress(t *testing.T) {
	t.Run("из флага", func(t *testing.T) {
		clearEnv(t)

		_, err := Parse([]string{"-d", testDSN, "-r", "htp://localhost:8081"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "ACCRUAL_SYSTEM_ADDRESS")
	})

	t.Run("из окружения", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "ftp://accrual.example")

		_, err := Parse([]string{"-d", testDSN})
		require.Error(t, err)
		assert.ErrorContains(t, err, "ACCRUAL_SYSTEM_ADDRESS")
	})
}

func TestParseDefaultSecret(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{name: "секрет не задан", want: true},
		{name: "секрет из флага", args: []string{"-s", "секрет из флага"}},
		{name: "секрет из окружения", env: "секрет из окружения"},
		{name: "явно передан секрет по умолчанию", args: []string{"-s", defaultJWTSecret}, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("JWT_SECRET", c.env)

			cfg, err := Parse(append([]string{"-d", testDSN}, c.args...))
			require.NoError(t, err)
			assert.Equal(t, c.want, cfg.UsesDefaultSecret())
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "без схемы", in: "localhost:8081", want: "http://localhost:8081"},
		{name: "со слешем в конце", in: "http://localhost:8081/", want: "http://localhost:8081"},
		{name: "https не трогаем", in: "https://accrual.example", want: "https://accrual.example"},
		{name: "пробелы по краям", in: "  localhost:8081  ", want: "http://localhost:8081"},
		{name: "пусто", in: "", want: ""},
		{name: "опечатка в схеме", in: "htp://localhost:8081", wantErr: true},
		{name: "чужая схема", in: "ftp://accrual.example", wantErr: true},
		{name: "нет хоста", in: "http://", wantErr: true},
		{name: "нет хоста, только порт", in: "http://:8081", wantErr: true},
		{name: "пробел в хосте", in: "local host:8081", wantErr: true},
		{name: "мусор", in: "%%%", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeURL(c.in)
			if c.wantErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, "-r")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
