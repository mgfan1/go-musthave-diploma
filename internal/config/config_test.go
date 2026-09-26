package config

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://gophermart:gophermart@localhost:5432/gophermart"

func withArgs(t *testing.T, args ...string) {
	t.Helper()

	for _, name := range []string{"RUN_ADDRESS", "DATABASE_URI", "ACCRUAL_SYSTEM_ADDRESS", "JWT_SECRET"} {
		t.Setenv(name, "")
	}

	oldArgs, oldFlags := os.Args, flag.CommandLine
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
	})

	os.Args = append([]string{"gophermart"}, args...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}

func TestParseDefaults(t *testing.T) {
	withArgs(t, "-d", testDSN)

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, "localhost:8080", cfg.Addr)
	assert.Equal(t, testDSN, cfg.DatabaseURI)
	assert.Empty(t, cfg.AccrualAddress)
	assert.Equal(t, defaultJWTSecret, cfg.JWTSecret)
}

func TestParseFlags(t *testing.T) {
	withArgs(t, "-a", ":9090", "-d", testDSN, "-r", "http://localhost:8081", "-s", "секрет из флага")

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, Config{
		Addr:           ":9090",
		DatabaseURI:    testDSN,
		AccrualAddress: "http://localhost:8081",
		JWTSecret:      "секрет из флага",
	}, cfg)
}

func TestParseEnvBeatsFlag(t *testing.T) {
	withArgs(t, "-a", ":9090", "-d", "postgres://flag", "-r", "http://flag:1", "-s", "флаг")
	t.Setenv("RUN_ADDRESS", "localhost:8081")
	t.Setenv("DATABASE_URI", testDSN)
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://localhost:8082")
	t.Setenv("JWT_SECRET", "секрет из окружения")

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, Config{
		Addr:           "localhost:8081",
		DatabaseURI:    testDSN,
		AccrualAddress: "http://localhost:8082",
		JWTSecret:      "секрет из окружения",
	}, cfg)
}

func TestParseEmptyEnvKeepsFlag(t *testing.T) {
	withArgs(t, "-a", ":9090", "-d", testDSN)
	t.Setenv("RUN_ADDRESS", "")

	cfg, err := Parse()
	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Addr)
}

func TestParseRequiresDatabase(t *testing.T) {
	withArgs(t)

	_, err := Parse()
	require.Error(t, err)
	assert.ErrorContains(t, err, "DATABASE_URI")
}

func TestParseRequiresSecret(t *testing.T) {
	withArgs(t, "-d", testDSN, "-s", "")

	_, err := Parse()
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET")
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"без схемы", "localhost:8081", "http://localhost:8081"},
		{"со слешем в конце", "http://localhost:8081/", "http://localhost:8081"},
		{"https не трогаем", "https://accrual.example", "https://accrual.example"},
		{"пробелы по краям", "  localhost:8081  ", "http://localhost:8081"},
		{"пусто", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, normalizeURL(c.in))
		})
	}
}
