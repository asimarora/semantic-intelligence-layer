package access

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"

        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        "gopkg.in/yaml.v3"
)

const (
        Kind                 = "access"
        Source               = "aaa_auth"
        DefaultSchemaVersion = "sil.evidence.access.v1"

        configFileName = "access.yaml"
)

type Config struct {
        ID            string `yaml:"id"`
        Kind          string `yaml:"kind"`
        Enabled       bool   `yaml:"enabled"`
        TenantID      string `yaml:"tenant_id"`
        DisplayName   string `yaml:"display_name"`
        Provider      string `yaml:"provider"`
        InputPath     string `yaml:"input_path"`
        SchemaVersion string `yaml:"schema_version"`
        Subject       string `yaml:"subject"`
}

func SupportsSourceType(value string) bool {
        switch strings.ToLower(strings.TrimSpace(value)) {
        case Source, Kind, "aaa", "aaa-auth":
                return true
        default:
                return false
        }
}

func LoadConfig(configDir string) (Config, error) {
        path := filepath.Join(strings.TrimSpace(configDir), configFileName)
        data, err := os.ReadFile(path)
        if err != nil {
                return Config{}, fmt.Errorf("read access config %s: %w", path, err)
        }

        var cfg Config
        if err := yaml.Unmarshal(data, &cfg); err != nil {
                return Config{}, fmt.Errorf("parse access config %s: %w", path, err)
        }

        if strings.TrimSpace(cfg.ID) == "" {
                cfg.ID = Kind
        }
        if strings.TrimSpace(cfg.Kind) == "" {
                cfg.Kind = Kind
        }
        if strings.TrimSpace(cfg.SchemaVersion) == "" {
                cfg.SchemaVersion = DefaultSchemaVersion
        }
        if strings.TrimSpace(cfg.Subject) == "" {
                cfg.Subject = messaging.SubjectRawEvents
        }
        if err := cfg.Validate(); err != nil {
                return Config{}, err
        }
        if !cfg.Enabled {
                return Config{}, fmt.Errorf("access source is disabled")
        }

        return cfg, nil
}

func (cfg Config) Validate() error {
        if strings.TrimSpace(cfg.ID) == "" {
                return fmt.Errorf("access config id is required")
        }
        if strings.TrimSpace(cfg.Kind) != Kind {
                return fmt.Errorf("access config kind must be %q", Kind)
        }
        if strings.TrimSpace(cfg.TenantID) == "" {
                return fmt.Errorf("access config tenant_id is required")
        }
        if strings.TrimSpace(cfg.Provider) == "" {
                return fmt.Errorf("access config provider is required")
        }
        if strings.TrimSpace(cfg.InputPath) == "" {
                return fmt.Errorf("access config input_path is required")
        }
        if strings.TrimSpace(cfg.SchemaVersion) == "" {
                return fmt.Errorf("access config schema_version is required")
        }
        if strings.TrimSpace(cfg.Subject) == "" {
                return fmt.Errorf("access config subject is required")
        }
        return nil
}

func (cfg Config) EffectiveInputPath(override string) string {
        override = strings.TrimSpace(override)
        if override != "" {
                return override
        }
        return strings.TrimSpace(cfg.InputPath)
}
