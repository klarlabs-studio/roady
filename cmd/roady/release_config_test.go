package main

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// The Homebrew cask links every binary it lists, and brew aborts the install
// or upgrade on the first one the archive does not contain. goreleaser does
// not check the list against the builds, so this does.
func TestReleaseCaskShipsOnlyBuiltBinaries(t *testing.T) {
	raw, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Builds []struct {
			Binary string `yaml:"binary"`
		} `yaml:"builds"`
		Casks []struct {
			Name     string   `yaml:"name"`
			Binaries []string `yaml:"binaries"`
		} `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	built := map[string]bool{}
	for _, b := range cfg.Builds {
		built[b.Binary] = true
	}
	if len(cfg.Casks) == 0 {
		t.Fatal("no homebrew_casks in .goreleaser.yaml")
	}
	for _, c := range cfg.Casks {
		for _, bin := range c.Binaries {
			if !built[bin] {
				t.Errorf("cask %s links %s, which no build produces", c.Name, bin)
			}
		}
	}
}
