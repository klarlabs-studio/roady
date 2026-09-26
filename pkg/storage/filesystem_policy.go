package storage

import (
	"bytes"
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"gopkg.in/yaml.v3"
)

func (r *FilesystemRepository) LoadPolicy() (*domain.PolicyConfig, error) {
	path, err := r.ResolvePath(PolicyFile)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- Path is resolved and validated via ResolvePath
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &domain.PolicyConfig{MaxWIP: 3, AllowAI: true}, nil // Default
		}
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	data = withoutRemovedPolicyKeys(data)
	var cfg domain.PolicyConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err == nil {
		return &cfg, nil
	}

	// Legacy policy support for deprecated provider/model fields.
	type legacyPolicyConfig struct {
		MaxWIP     int    `yaml:"max_wip"`
		AllowAI    bool   `yaml:"allow_ai"`
		TokenLimit int    `yaml:"token_limit"` // ignored: roady runs no inference
		AIProvider string `yaml:"ai_provider"`
		AIModel    string `yaml:"ai_model"`
	}

	var legacy legacyPolicyConfig
	decLegacy := yaml.NewDecoder(bytes.NewReader(data))
	decLegacy.KnownFields(true)
	if err := decLegacy.Decode(&legacy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal policy: %w", err)
	}

	return &domain.PolicyConfig{
		MaxWIP:  legacy.MaxWIP,
		AllowAI: legacy.AllowAI,
	}, nil
}

func (r *FilesystemRepository) SavePolicy(cfg *domain.PolicyConfig) error {
	path, err := r.ResolvePath(PolicyFile)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// removedPolicyKeys were policy settings for features roady no longer has
// (budgets, team roles, an AI token budget — roady runs no inference). A policy.yaml that still sets them loads as if they
// were absent, rather than failing the strict decode below.
var removedPolicyKeys = map[string]bool{"budget_hours": true, "enforce_team_roles": true, "token_limit": true}

func withoutRemovedPolicyKeys(data []byte) []byte {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data
	}
	m := doc.Content[0]
	kept := m.Content[:0]
	removed := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		if removedPolicyKeys[m.Content[i].Value] {
			removed = true
			continue
		}
		kept = append(kept, m.Content[i], m.Content[i+1])
	}
	if !removed {
		return data
	}
	m.Content = kept
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return data
	}
	return out
}
