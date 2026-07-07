//go:generate go run ./gen

package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultMetricsHost = "0.0.0.0"
	DefaultMetricsPort = 9090
	DefaultMetricsPath = "/metrics"
)

// Load parses configuration from command-line flags and environment variables.
// Flags take precedence over environment variables, which take precedence over defaults.
func Load() (Config, error) {
	var cfg Config
	fs := newFlagSet(&cfg)

	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return Config{}, err
	}

	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	if cfg.MetricsPort < 1 || cfg.MetricsPort > 65535 {
		return Config{}, fmt.Errorf("--metrics-port must be between 1 and 65535, got %d", cfg.MetricsPort)
	}
	if !strings.HasPrefix(cfg.MetricsPath, "/") {
		return Config{}, fmt.Errorf("--metrics-path must start with /, got %q", cfg.MetricsPath)
	}

	return cfg, nil
}

func printHelp() {
	fmt.Fprintf(os.Stderr, "Usage: lostromos [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Flags and environment variables (flags take precedence over env vars):\n\n")
	for _, e := range helpEntries {
		if e.env != "" {
			fmt.Fprintf(os.Stderr, "  %-30s env: %-30s default: %s\n      %s\n", e.flag, e.env, e.def, e.desc)
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n      %s\n", e.flag, e.desc)
		}
	}
}

func boolEnv(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "true" || v == "1" || v == "yes"
}

func strEnv(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func intEnv(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

