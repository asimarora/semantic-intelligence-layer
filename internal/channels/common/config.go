package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ID          string `yaml:"id"`
	Kind        string `yaml:"kind"`
	Enabled     bool   `yaml:"enabled"`
	TenantID    string `yaml:"tenant_id"`
	DisplayName string `yaml:"display_name"`
	Provider    string `yaml:"provider"`
	WebhookPath string `yaml:"webhook_path"`
}

type Registry struct {
	Configs []Config
}

func LoadConfigs(configDir string, enabled []string, basePath string) (Registry, error) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		return Registry{}, nil
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{}, nil
		}
		return Registry{}, fmt.Errorf("read channel config dir %s: %w", configDir, err)
	}

	enabledSet := make(map[string]struct{}, len(enabled))
	for _, value := range enabled {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			enabledSet[value] = struct{}{}
		}
	}

	registry := Registry{Configs: make([]Config, 0, len(entries))}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(configDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return Registry{}, fmt.Errorf("read channel config %s: %w", path, err)
		}

		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Registry{}, fmt.Errorf("parse channel config %s: %w", path, err)
		}
		cfg.ID = strings.TrimSpace(cfg.ID)
		cfg.Kind = strings.TrimSpace(cfg.Kind)
		if cfg.ID == "" {
			cfg.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if cfg.Kind == "" {
			cfg.Kind = cfg.ID
		}
		if cfg.WebhookPath == "" && strings.TrimSpace(basePath) != "" {
			cfg.WebhookPath = strings.TrimRight(strings.TrimSpace(basePath), "/") + "/" + cfg.ID + "/webhook"
		}
		if _, ok := enabledSet[strings.ToLower(cfg.ID)]; len(enabledSet) > 0 && !ok {
			continue
		}
		if !cfg.Enabled {
			continue
		}
		if cfg.WebhookPath == "" {
			return Registry{}, fmt.Errorf("channel %s webhook_path is required", cfg.ID)
		}
		registry.Configs = append(registry.Configs, cfg)
	}

	sort.Slice(registry.Configs, func(left, right int) bool {
		return registry.Configs[left].ID < registry.Configs[right].ID
	})
	return registry, nil
}
