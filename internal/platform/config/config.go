package config

import (
        "fmt"
        "os"
        "strconv"
        "strings"

        "gopkg.in/yaml.v3"
)

type rawConfig struct {
        App      *rawApp      `yaml:"app"`
        Logging  *rawLogging  `yaml:"logging"`
        Pipeline *rawPipeline `yaml:"pipeline"`
        Ingest   *rawIngest   `yaml:"ingest"`
        Storage  *rawStorage  `yaml:"storage"`
        Stream   *rawStream   `yaml:"stream"`
        Cache    *rawCache    `yaml:"cache"`
}

type rawApp struct {
        Name *string `yaml:"name"`
        Env  *string `yaml:"env"`
}

type rawLogging struct {
        Level     *string `yaml:"level"`
        Format    *string `yaml:"format"`
        AddSource *bool   `yaml:"add_source"`
}

type rawPipeline struct {
        Workers   *int `yaml:"workers"`
        QueueSize *int `yaml:"queue_size"`
        BatchSize *int `yaml:"batch_size"`
}

type rawIngest struct {
        SourceType *string `yaml:"source_type"`
        InputPath  *string `yaml:"input_path"`
}

type rawStorage struct {
        Raw      *rawStore `yaml:"raw"`
        Metadata *rawStore `yaml:"metadata"`
        Vector   *rawStore `yaml:"vector"`
}

type rawStore struct {
        Backend    *string `yaml:"backend"`
        Path       *string `yaml:"path"`
        Addr       *string `yaml:"addr"`
        Database   *string `yaml:"database"`
        Collection *string `yaml:"collection"`
}

type rawStream struct {
        Backend *string `yaml:"backend"`
        Addr    *string `yaml:"addr"`
        Subject *string `yaml:"subject"`
}

type rawCache struct {
        Backend *string `yaml:"backend"`
        Addr    *string `yaml:"addr"`
}

type Config struct {
        App      AppConfig
        Logging  LoggingConfig
        Pipeline PipelineConfig
        Ingest   IngestConfig
        Storage  StorageConfig
        Stream   StreamConfig
        Cache    CacheConfig
}

type AppConfig struct {
        Name string
        Env  string
}

type LoggingConfig struct {
        Level     string
        Format    string
        AddSource bool
}

type PipelineConfig struct {
        Workers   int
        QueueSize int
        BatchSize int
}

type IngestConfig struct {
        SourceType string
        InputPath  string
}

type StorageConfig struct {
        Raw      StoreConfig
        Metadata StoreConfig
        Vector   StoreConfig
}

type StoreConfig struct {
        Backend    string
        Path       string
        Addr       string
        Database   string
        Collection string
}

type StreamConfig struct {
        Backend string
        Addr    string
        Subject string
}

type CacheConfig struct {
        Backend string
        Addr    string
}

func Load() (*Config, error) {
        env := normalizeEnvName(envOrFirst([]string{"SIL_APP_ENV", "APP_ENV"}, "development"))
        configDir := envOrFirst([]string{"SIL_CONFIG_DIR", "CONFIG_DIR"}, "./configs")
        return LoadFrom(configDir, env)
}

func LoadFrom(configDir, env string) (*Config, error) {
        env = normalizeEnvName(env)

        base, err := loadYAML(configDir + "/base.yaml")
        if err != nil && !os.IsNotExist(err) {
                return nil, fmt.Errorf("load base config: %w", err)
        }

        envCfg, err := loadYAML(configDir + "/" + env + ".yaml")
        if err != nil && !os.IsNotExist(err) {
                return nil, fmt.Errorf("load %s config: %w", env, err)
        }

        cfg := resolve(env, merge(base, envCfg))
        return cfg, cfg.validate()
}

func loadYAML(path string) (*rawConfig, error) {
        data, err := os.ReadFile(path)
        if err != nil {
                return nil, err
        }

        var cfg rawConfig
        if err := yaml.Unmarshal(data, &cfg); err != nil {
                return nil, fmt.Errorf("parse %s: %w", path, err)
        }

        return &cfg, nil
}

func merge(base, overlay *rawConfig) *rawConfig {
        if base == nil {
                base = &rawConfig{}
        }
        if overlay == nil {
                return base
        }

        result := *base

        if overlay.App != nil {
                if result.App == nil {
                        result.App = &rawApp{}
                }
                mergeApp(result.App, overlay.App)
        }

        if overlay.Logging != nil {
                if result.Logging == nil {
                        result.Logging = &rawLogging{}
                }
                mergeLogging(result.Logging, overlay.Logging)
        }

        if overlay.Pipeline != nil {
                if result.Pipeline == nil {
                        result.Pipeline = &rawPipeline{}
                }
                mergePipeline(result.Pipeline, overlay.Pipeline)
        }

        if overlay.Ingest != nil {
                if result.Ingest == nil {
                        result.Ingest = &rawIngest{}
                }
                mergeIngest(result.Ingest, overlay.Ingest)
        }

        if overlay.Storage != nil {
                if result.Storage == nil {
                        result.Storage = &rawStorage{}
                }
                mergeStorage(result.Storage, overlay.Storage)
        }

        if overlay.Stream != nil {
                if result.Stream == nil {
                        result.Stream = &rawStream{}
                }
                mergeStream(result.Stream, overlay.Stream)
        }

        if overlay.Cache != nil {
                if result.Cache == nil {
                        result.Cache = &rawCache{}
                }
                mergeCache(result.Cache, overlay.Cache)
        }

        return &result
}

func mergeApp(dst, src *rawApp) {
        if src.Name != nil {
                dst.Name = src.Name
        }
        if src.Env != nil {
                dst.Env = src.Env
        }
}

func mergeLogging(dst, src *rawLogging) {
        if src.Level != nil {
                dst.Level = src.Level
        }
        if src.Format != nil {
                dst.Format = src.Format
        }
        if src.AddSource != nil {
                dst.AddSource = src.AddSource
        }
}

func mergePipeline(dst, src *rawPipeline) {
        if src.Workers != nil {
                dst.Workers = src.Workers
        }
        if src.QueueSize != nil {
                dst.QueueSize = src.QueueSize
        }
        if src.BatchSize != nil {
                dst.BatchSize = src.BatchSize
        }
}

func mergeIngest(dst, src *rawIngest) {
        if src.SourceType != nil {
                dst.SourceType = src.SourceType
        }
        if src.InputPath != nil {
                dst.InputPath = src.InputPath
        }
}

func mergeStorage(dst, src *rawStorage) {
        if src.Raw != nil {
                if dst.Raw == nil {
                        dst.Raw = &rawStore{}
                }
                mergeStore(dst.Raw, src.Raw)
        }
        if src.Metadata != nil {
                if dst.Metadata == nil {
                        dst.Metadata = &rawStore{}
                }
                mergeStore(dst.Metadata, src.Metadata)
        }
        if src.Vector != nil {
                if dst.Vector == nil {
                        dst.Vector = &rawStore{}
                }
                mergeStore(dst.Vector, src.Vector)
        }
}

func mergeStore(dst, src *rawStore) {
        if src.Backend != nil {
                dst.Backend = src.Backend
        }
        if src.Path != nil {
                dst.Path = src.Path
        }
        if src.Addr != nil {
                dst.Addr = src.Addr
        }
        if src.Database != nil {
                dst.Database = src.Database
        }
        if src.Collection != nil {
                dst.Collection = src.Collection
        }
}

func mergeStream(dst, src *rawStream) {
        if src.Backend != nil {
                dst.Backend = src.Backend
        }
        if src.Addr != nil {
                dst.Addr = src.Addr
        }
        if src.Subject != nil {
                dst.Subject = src.Subject
        }
}

func mergeCache(dst, src *rawCache) {
        if src.Backend != nil {
                dst.Backend = src.Backend
        }
        if src.Addr != nil {
                dst.Addr = src.Addr
        }
}

func resolve(env string, raw *rawConfig) *Config {
        if raw == nil {
                raw = &rawConfig{}
        }

        return &Config{
                App: AppConfig{
                        Name: envOrFirst([]string{"SIL_APP_NAME"}, ptrOr(safe(raw.App).Name, "semantic-intelligence-layer")),
                        Env:  ptrOr(safe(raw.App).Env, env),
                },
                Logging: LoggingConfig{
                        Level:     envOrFirst([]string{"SIL_LOG_LEVEL"}, ptrOr(safe(raw.Logging).Level, "info")),
                        Format:    envOrFirst([]string{"SIL_LOG_FORMAT"}, ptrOr(safe(raw.Logging).Format, "text")),
                        AddSource: boolOrFirst([]string{"SIL_LOG_ADD_SOURCE"}, ptrOr(safe(raw.Logging).AddSource, false)),
                },
                Pipeline: PipelineConfig{
                        Workers:   intOrFirst([]string{"SIL_PIPELINE_WORKERS"}, ptrOr(safe(raw.Pipeline).Workers, 2)),
                        QueueSize: intOrFirst([]string{"SIL_PIPELINE_QUEUE_SIZE"}, ptrOr(safe(raw.Pipeline).QueueSize, 128)),
                        BatchSize: intOrFirst([]string{"SIL_PIPELINE_BATCH_SIZE"}, ptrOr(safe(raw.Pipeline).BatchSize, 1)),
                },
                Ingest: IngestConfig{
                        SourceType: envOrFirst([]string{"SIL_INGEST_SOURCE_TYPE"}, ptrOr(safe(raw.Ingest).SourceType, "ras")),
                        InputPath:  envOrFirst([]string{"SIL_INGEST_INPUT_PATH"}, ptrOr(safe(raw.Ingest).InputPath, "./testdata/ras")),
                },
                Storage: StorageConfig{
                        Raw: StoreConfig{
                                Backend: envOrFirst([]string{"SIL_RAW_STORAGE_BACKEND"}, ptrOr(safe(safe(raw.Storage).Raw).Backend, "file")),
                                Path:    envOrFirst([]string{"SIL_RAW_STORAGE_PATH"}, ptrOr(safe(safe(raw.Storage).Raw).Path, "./var/raw")),
                                Addr:    envOrFirst([]string{"SIL_RAW_STORAGE_ADDR"}, ptrOr(safe(safe(raw.Storage).Raw).Addr, "")),
                        },
                        Metadata: StoreConfig{
                                Backend:  envOrFirst([]string{"SIL_METADATA_STORAGE_BACKEND"}, ptrOr(safe(safe(raw.Storage).Metadata).Backend, "memory")),
                                Path:     envOrFirst([]string{"SIL_METADATA_STORAGE_PATH"}, ptrOr(safe(safe(raw.Storage).Metadata).Path, "")),
                                Addr:     envOrFirst([]string{"SIL_METADATA_STORAGE_ADDR"}, ptrOr(safe(safe(raw.Storage).Metadata).Addr, "")),
                                Database: envOrFirst([]string{"SIL_METADATA_STORAGE_DATABASE"}, ptrOr(safe(safe(raw.Storage).Metadata).Database, "")),
                        },
                        Vector: StoreConfig{
                                Backend:    envOrFirst([]string{"SIL_VECTOR_STORAGE_BACKEND"}, ptrOr(safe(safe(raw.Storage).Vector).Backend, "memory")),
                                Path:       envOrFirst([]string{"SIL_VECTOR_STORAGE_PATH"}, ptrOr(safe(safe(raw.Storage).Vector).Path, "")),
                                Addr:       envOrFirst([]string{"SIL_VECTOR_STORAGE_ADDR"}, ptrOr(safe(safe(raw.Storage).Vector).Addr, "")),
                                Collection: envOrFirst([]string{"SIL_VECTOR_STORAGE_COLLECTION"}, ptrOr(safe(safe(raw.Storage).Vector).Collection, "")),
                        },
                },
                Stream: StreamConfig{
                        Backend: envOrFirst([]string{"SIL_STREAM_BACKEND"}, ptrOr(safe(raw.Stream).Backend, "disabled")),
                        Addr:    envOrFirst([]string{"SIL_STREAM_ADDR"}, ptrOr(safe(raw.Stream).Addr, "")),
                        Subject: envOrFirst([]string{"SIL_STREAM_SUBJECT"}, ptrOr(safe(raw.Stream).Subject, "sil.raw.events")),
                },
                Cache: CacheConfig{
                        Backend: envOrFirst([]string{"SIL_CACHE_BACKEND"}, ptrOr(safe(raw.Cache).Backend, "disabled")),
                        Addr:    envOrFirst([]string{"SIL_CACHE_ADDR"}, ptrOr(safe(raw.Cache).Addr, "")),
                },
        }
}

func (c *Config) validate() error {
        if strings.TrimSpace(c.App.Name) == "" {
                return fmt.Errorf("app.name is required")
        }
        if strings.TrimSpace(c.App.Env) == "" {
                return fmt.Errorf("app.env is required")
        }
        if !validLogLevel(c.Logging.Level) {
                return fmt.Errorf("unsupported logging.level %q", c.Logging.Level)
        }
        if !validLogFormat(c.Logging.Format) {
                return fmt.Errorf("unsupported logging.format %q", c.Logging.Format)
        }
        if c.Pipeline.Workers <= 0 {
                return fmt.Errorf("pipeline.workers must be positive")
        }
        if c.Pipeline.QueueSize <= 0 {
                return fmt.Errorf("pipeline.queue_size must be positive")
        }
        if c.Pipeline.BatchSize <= 0 {
                return fmt.Errorf("pipeline.batch_size must be positive")
        }
        if strings.TrimSpace(c.Ingest.SourceType) == "" {
                return fmt.Errorf("ingest.source_type is required")
        }
        if strings.TrimSpace(c.Ingest.InputPath) == "" {
                return fmt.Errorf("ingest.input_path is required")
        }
        if err := validateStore("storage.raw", c.Storage.Raw, false); err != nil {
                return err
        }
        if err := validateStore("storage.metadata", c.Storage.Metadata, false); err != nil {
                return err
        }
        if err := validateStore("storage.vector", c.Storage.Vector, true); err != nil {
                return err
        }
        if !isDisabled(c.Stream.Backend) {
                if strings.TrimSpace(c.Stream.Addr) == "" {
                        return fmt.Errorf("stream.addr is required when stream backend is %q", c.Stream.Backend)
                }
                if strings.TrimSpace(c.Stream.Subject) == "" {
                        return fmt.Errorf("stream.subject is required when stream backend is %q", c.Stream.Backend)
                }
        }
        if !isDisabled(c.Cache.Backend) && strings.TrimSpace(c.Cache.Addr) == "" {
                return fmt.Errorf("cache.addr is required when cache backend is %q", c.Cache.Backend)
        }
        return nil
}

func validateStore(name string, cfg StoreConfig, requireCollection bool) error {
        if strings.TrimSpace(cfg.Backend) == "" {
                return fmt.Errorf("%s.backend is required", name)
        }

        switch normalized := strings.ToLower(strings.TrimSpace(cfg.Backend)); normalized {
        case "file", "sqlite":
                if strings.TrimSpace(cfg.Path) == "" {
                        return fmt.Errorf("%s.path is required when backend is %q", name, cfg.Backend)
                }
        case "disabled", "memory":
        default:
                if strings.TrimSpace(cfg.Addr) == "" {
                        return fmt.Errorf("%s.addr is required when backend is %q", name, cfg.Backend)
                }
        }

        if requireCollection && !isDisabled(cfg.Backend) && !isMemory(cfg.Backend) && strings.TrimSpace(cfg.Collection) == "" {
                return fmt.Errorf("%s.collection is required when backend is %q", name, cfg.Backend)
        }

        return nil
}

func validLogLevel(level string) bool {
        switch strings.ToLower(strings.TrimSpace(level)) {
        case "debug", "info", "warn", "error":
                return true
        default:
                return false
        }
}

func validLogFormat(format string) bool {
        switch strings.ToLower(strings.TrimSpace(format)) {
        case "text", "json":
                return true
        default:
                return false
        }
}

func isDisabled(value string) bool {
        return strings.EqualFold(strings.TrimSpace(value), "disabled")
}

func isMemory(value string) bool {
        return strings.EqualFold(strings.TrimSpace(value), "memory")
}

func envOrFirst(keys []string, fallback string) string {
        for _, key := range keys {
                if value, ok := os.LookupEnv(key); ok {
                        value = strings.TrimSpace(value)
                        if value != "" {
                                return value
                        }
                }
        }
        return fallback
}

func normalizeEnvName(value string) string {
        switch strings.ToLower(strings.TrimSpace(value)) {
        case "", "development", "dev":
                return "development"
        case "testing", "test":
                return "testing"
        case "staging", "stage", "stg", "qa":
                return "staging"
        case "production", "prod", "prd":
                return "production"
        default:
                return strings.TrimSpace(value)
        }
}

func intOrFirst(keys []string, fallback int) int {
        for _, key := range keys {
                if value, ok := os.LookupEnv(key); ok {
                        value = strings.TrimSpace(value)
                        if value == "" {
                                continue
                        }
                        parsed, err := strconv.Atoi(value)
                        if err == nil {
                                return parsed
                        }
                }
        }
        return fallback
}

func boolOrFirst(keys []string, fallback bool) bool {
        for _, key := range keys {
                if value, ok := os.LookupEnv(key); ok {
                        value = strings.TrimSpace(value)
                        if value == "" {
                                continue
                        }
                        parsed, err := strconv.ParseBool(value)
                        if err == nil {
                                return parsed
                        }
                }
        }
        return fallback
}

func ptrOr[T any](value *T, fallback T) T {
        if value != nil {
                return *value
        }
        return fallback
}

func safe[T any](value *T) *T {
        if value != nil {
                return value
        }
        var zero T
        return &zero
}
