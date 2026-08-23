// Package config layers CLI flags, IRONSTATE_* env vars, and an optional
// ironstate.yaml/.ironstate.yaml config file (via viper) into one typed
// Config struct — see docs/plans/go-rewrite.md §4.1. Nothing outside
// internal/cli touches viper directly.
package config

import (
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the fully-resolved set of options every subcommand reads.
type Config struct {
	File    string   // site.yml path (--file, alias --packages-file)
	Apply   bool     // --apply
	Tags    []string // --tags
	Output  string   // --output table|json
	Verbose bool     // -v/--verbose

	FiltersDir         string              // directory scanned for external script filters
	FilterInterpreters map[string][]string // script extension -> interpreter argv prefix
}

// Load resolves Config from flags (highest precedence), IRONSTATE_* env
// vars, an optional ironstate.yaml/.ironstate.yaml in the working
// directory, then built-in defaults.
func Load(flags *pflag.FlagSet) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("IRONSTATE")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetConfigName("ironstate")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, err
		}
	}

	v.SetDefault("file", "site.yml")
	v.SetDefault("output", "table")
	v.SetDefault("filters.dir", "modules/Filters")

	if err := v.BindPFlags(flags); err != nil {
		return nil, err
	}

	var interpreters map[string][]string
	if err := v.UnmarshalKey("filters.interpreters", &interpreters); err != nil {
		return nil, err
	}

	return &Config{
		File:    v.GetString("file"),
		Apply:   v.GetBool("apply"),
		Tags:    v.GetStringSlice("tags"),
		Output:  v.GetString("output"),
		Verbose: v.GetBool("verbose"),

		FiltersDir:         v.GetString("filters.dir"),
		FilterInterpreters: interpreters,
	}, nil
}
