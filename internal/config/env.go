package config

import "os"

func envString(name string, dst *string) {
	if v := os.Getenv(name); v != "" {
		*dst = v
	}
}
