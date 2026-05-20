package config

import (
        "fmt"
        "os"
        "strconv"
        "strings"
        "time"

        "gopkg.in/yaml.v3"
)

type rawConfig struct {
        App      *rawApp      `yaml:"app"`
        API      *rawAPI      `yaml:"api"`
        Logging  *rawLogging  `yaml:"logging"`
        Pipeline *rawPipeline `yaml:"pipeline"`
        Ingest   *rawIngest   `yaml:"ingest"`
        Sources  *rawSources  `yaml:"sources"`
        Channels *rawChannels `yaml:"channels"`
        Storage  *rawStorage  `yaml:"storage"`
        Stream   *rawStream   `yaml:"stream"`
        Cache    *rawCache    `yaml:"cache"`
}

type rawApp struct {
        Name *string `yaml:"name"`
        Env  *string `yaml:"env"`
}

type rawAPI struct {
        ListenAddr   *string `yaml:"listen_addr"`
        ReadTimeout  *string `yaml:"read_timeout"`
        WriteTimeout *string `yaml:"write_timeout"`
        IdleTimeout  *string `yaml:"idle_timeout"`
}

type rawLogging struct {
        Level          *string `yaml:"level"`
        Format         *string `yaml:"format"`
        AddSource      *bool   `yaml:"add_source"`
        AsyncEnabled   *bool   `yaml:"async_enabled"`
        AsyncQueueSize *int    `yaml:"async_queue_size"`
        FlushTimeout   *string `yaml:"flush_timeout"`
        OutputDir      *string `yaml:"output_dir"`
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

type rawSources struct {
        ConfigDir *string  `yaml:"config_dir"`
        Enabled   []string `yaml:"enabled"`
}

type rawChannels struct {
        ConfigDir             *string  `yaml:"config_dir"`
        Enabled               []string `yaml:"enabled"`
        WebhookBasePath       *string  `yaml:"webhook_base_path"`
        RequestBodyLimitBytes *int     `yaml:"request_body_limit_bytes"`
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
        Backend           *string `yaml:"backend"`
        Addr              *string `yaml:"addr"`
        StreamName        *string `yaml:"stream_name"`
        Storage           *string `yaml:"storage"`
        RawSubject        *string `yaml:"raw_subject"`
        UnifiedSubject    *string `yaml:"unified_subject"`
        IndexSubject      *string `yaml:"index_subject"`
        DeadLetterSubject *string `yaml:"deadletter_subject"`
        ConsumerPrefix    *string `yaml:"consumer_prefix"`
        AckWait           *string `yaml:"ack_wait"`
        MaxDeliveries     *int    `yaml:"max_deliveries"`
        BatchSize         *int    `yaml:"batch_size"`
        MaxAckPending     *int    `yaml:"max_ack_pending"`
        ConnectTimeout    *string `yaml:"connect_timeout"`
        Username          *string `yaml:"username"`
        Password          *string `yaml:"password"`
        TLSEnabled        *bool   `yaml:"tls_enabled"`
}

type rawCache struct {
        Backend *string `yaml:"backend"`
        Addr    *string `yaml:"addr"`
}

type Config struct {
        App      AppConfig
        API      APIConfig
        Logging  LoggingConfig
        Pipeline PipelineConfig
        Ingest   IngestConfig
        Sources  SourcesConfig
        Channels ChannelsConfig
        Storage  StorageConfig
        Stream   StreamConfig
        Cache    CacheConfig
}

type AppConfig struct {
        Name string
        Env  string
}

type APIConfig struct {
        ListenAddr   string
        ReadTimeout  time.Duration
        WriteTimeout time.Duration
        IdleTimeout  time.Duration
}

type LoggingConfig struct {
        Level          string
        Format         string
        AddSource      bool
        AsyncEnabled   bool
        AsyncQueueSize int
        FlushTimeout   time.Duration
        OutputDir      string
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

type SourcesConfig struct {
        ConfigDir string
        Enabled   []string
}

type ChannelsConfig struct {
        ConfigDir             string
        Enabled               []string
        WebhookBasePath       string
        RequestBodyLimitBytes int
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
        Backend           string
        Addr              string
        StreamName        string
        Storage           string
        RawSubject        string
        UnifiedSubject    string
        IndexSubject      string
        DeadLetterSubject string
        ConsumerPrefix    string
        AckWait           time.Duration
        MaxDeliveries     int
        BatchSize         int
        MaxAckPending     int
        ConnectTimeout    time.Duration
        Username          string
        Password          string
        TLSEnabled        bool
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

        if overlay.API != nil {
                if result.API == nil {
                        result.API = &rawAPI{}
                }
                mergeAPI(result.API, overlay.API)
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

        if overlay.Sources != nil {
                if result.Sources == nil {
                        result.Sources = &rawSources{}
                }
                mergeSources(result.Sources, overlay.Sources)
        }

        if overlay.Channels != nil {
                if result.Channels == nil {
                        result.Channels = &rawChannels{}
                }
                mergeChannels(result.Channels, overlay.Channels)
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

func mergeAPI(dst, src *rawAPI) {
        if src.ListenAddr != nil {
                dst.ListenAddr = src.ListenAddr
        }
        if src.ReadTimeout != nil {
                dst.ReadTimeout = src.ReadTimeout
        }
        if src.WriteTimeout != nil {
                dst.WriteTimeout = src.WriteTimeout
        }
        if src.IdleTimeout != nil {
                dst.IdleTimeout = src.IdleTimeout
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
        if src.AsyncEnabled != nil {
                dst.AsyncEnabled = src.AsyncEnabled
        }
        if src.AsyncQueueSize != nil {
                dst.AsyncQueueSize = src.AsyncQueueSize
        }
        if src.FlushTimeout != nil {
                dst.FlushTimeout = src.FlushTimeout
        }
        if src.OutputDir != nil {
                dst.OutputDir = src.OutputDir
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

func mergeSources(dst, src *rawSources) {
        if src.ConfigDir != nil {
                dst.ConfigDir = src.ConfigDir
        }
        if len(src.Enabled) > 0 {
                dst.Enabled = append([]string(nil), src.Enabled...)
        }
}

func mergeChannels(dst, src *rawChannels) {
        if src.ConfigDir != nil {
                dst.ConfigDir = src.ConfigDir
        }
        if len(src.Enabled) > 0 {
                dst.Enabled = append([]string(nil), src.Enabled...)
        }
        if src.WebhookBasePath != nil {
                dst.WebhookBasePath = src.WebhookBasePath
        }
        if src.RequestBodyLimitBytes != nil {
                dst.RequestBodyLimitBytes = src.RequestBodyLimitBytes
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
        if src.StreamName != nil {
                dst.StreamName = src.StreamName
        }
        if src.Storage != nil {
                dst.Storage = src.Storage
        }
        if src.RawSubject != nil {
                dst.RawSubject = src.RawSubject
        }
        if src.UnifiedSubject != nil {
                dst.UnifiedSubject = src.UnifiedSubject
        }
        if src.IndexSubject != nil {
                dst.IndexSubject = src.IndexSubject
        }
        if src.DeadLetterSubject != nil {
                dst.DeadLetterSubject = src.DeadLetterSubject
        }
        if src.ConsumerPrefix != nil {
                dst.ConsumerPrefix = src.ConsumerPrefix
        }
        if src.AckWait != nil {
                dst.AckWait = src.AckWait
        }
        if src.MaxDeliveries != nil {
                dst.MaxDeliveries = src.MaxDeliveries
        }
        if src.BatchSize != nil {
                dst.BatchSize = src.BatchSize
        }
        if src.MaxAckPending != nil {
                dst.MaxAckPending = src.MaxAckPending
        }
        if src.ConnectTimeout != nil {
                dst.ConnectTimeout = src.ConnectTimeout
        }
        if src.Username != nil {
                dst.Username = src.Username
        }
        if src.Password != nil {
                dst.Password = src.Password
        }
        if src.TLSEnabled != nil {
                dst.TLSEnabled = src.TLSEnabled
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
                API: APIConfig{
                        ListenAddr:   envOrFirst([]string{"SIL_API_LISTEN_ADDR"}, ptrOr(safe(raw.API).ListenAddr, ":8080")),
                        ReadTimeout:  durationOrFirst([]string{"SIL_API_READ_TIMEOUT"}, ptrOr(safe(raw.API).ReadTimeout, "5s")),
                        WriteTimeout: durationOrFirst([]string{"SIL_API_WRITE_TIMEOUT"}, ptrOr(safe(raw.API).WriteTimeout, "15s")),
                        IdleTimeout:  durationOrFirst([]string{"SIL_API_IDLE_TIMEOUT"}, ptrOr(safe(raw.API).IdleTimeout, "30s")),
                },
                Logging: LoggingConfig{
                        Level:          envOrFirst([]string{"SIL_LOG_LEVEL"}, ptrOr(safe(raw.Logging).Level, "info")),
                        Format:         envOrFirst([]string{"SIL_LOG_FORMAT"}, ptrOr(safe(raw.Logging).Format, "text")),
                        AddSource:      boolOrFirst([]string{"SIL_LOG_ADD_SOURCE"}, ptrOr(safe(raw.Logging).AddSource, false)),
                        AsyncEnabled:   boolOrFirst([]string{"SIL_LOG_ASYNC_ENABLED"}, ptrOr(safe(raw.Logging).AsyncEnabled, true)),
                        AsyncQueueSize: intOrFirst([]string{"SIL_LOG_ASYNC_QUEUE_SIZE"}, ptrOr(safe(raw.Logging).AsyncQueueSize, 1024)),
                        FlushTimeout:   durationOrFirst([]string{"SIL_LOG_FLUSH_TIMEOUT"}, ptrOr(safe(raw.Logging).FlushTimeout, "5s")),
                        OutputDir:      envOrFirst([]string{"SIL_LOG_OUTPUT_DIR"}, ptrOr(safe(raw.Logging).OutputDir, "")),
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
                Sources: SourcesConfig{
                        ConfigDir: envOrFirst([]string{"SIL_SOURCES_CONFIG_DIR"}, ptrOr(safe(raw.Sources).ConfigDir, "./configs/sources")),
                        Enabled:   listOrFirst([]string{"SIL_SOURCES_ENABLED"}, sliceOrDefault(safe(raw.Sources).Enabled, []string{"radius"})),
                },
                Channels: ChannelsConfig{
                        ConfigDir:             envOrFirst([]string{"SIL_CHANNELS_CONFIG_DIR"}, ptrOr(safe(raw.Channels).ConfigDir, "./configs/channels")),
                        Enabled:               listOrFirst([]string{"SIL_CHANNELS_ENABLED"}, sliceOrDefault(safe(raw.Channels).Enabled, []string{"chat", "email", "voice", "messaging"})),
                        WebhookBasePath:       envOrFirst([]string{"SIL_CHANNELS_WEBHOOK_BASE_PATH"}, ptrOr(safe(raw.Channels).WebhookBasePath, "/v1/channels")),
                        RequestBodyLimitBytes: intOrFirst([]string{"SIL_CHANNELS_REQUEST_BODY_LIMIT_BYTES"}, ptrOr(safe(raw.Channels).RequestBodyLimitBytes, 1<<20)),
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
                        Backend:           envOrFirst([]string{"SIL_STREAM_BACKEND"}, ptrOr(safe(raw.Stream).Backend, "disabled")),
                        Addr:              envOrFirst([]string{"SIL_STREAM_ADDR"}, ptrOr(safe(raw.Stream).Addr, "")),
                        StreamName:        envOrFirst([]string{"SIL_STREAM_NAME"}, ptrOr(safe(raw.Stream).StreamName, "SIL_EVENTS")),
                        Storage:           envOrFirst([]string{"SIL_STREAM_STORAGE"}, ptrOr(safe(raw.Stream).Storage, "file")),
                        RawSubject:        envOrFirst([]string{"SIL_STREAM_RAW_SUBJECT"}, ptrOr(safe(raw.Stream).RawSubject, "sil.raw.events")),
                        UnifiedSubject:    envOrFirst([]string{"SIL_STREAM_UNIFIED_SUBJECT"}, ptrOr(safe(raw.Stream).UnifiedSubject, "sil.unified.events")),
                        IndexSubject:      envOrFirst([]string{"SIL_STREAM_INDEX_SUBJECT"}, ptrOr(safe(raw.Stream).IndexSubject, "sil.index.jobs")),
                        DeadLetterSubject: envOrFirst([]string{"SIL_STREAM_DEADLETTER_SUBJECT"}, ptrOr(safe(raw.Stream).DeadLetterSubject, "sil.deadletter")),
                        ConsumerPrefix:    envOrFirst([]string{"SIL_STREAM_CONSUMER_PREFIX"}, ptrOr(safe(raw.Stream).ConsumerPrefix, "sil")),
                        AckWait:           durationOrFirst([]string{"SIL_STREAM_ACK_WAIT"}, ptrOr(safe(raw.Stream).AckWait, "30s")),
                        MaxDeliveries:     intOrFirst([]string{"SIL_STREAM_MAX_DELIVERIES"}, ptrOr(safe(raw.Stream).MaxDeliveries, 5)),
                        BatchSize:         intOrFirst([]string{"SIL_STREAM_BATCH_SIZE"}, ptrOr(safe(raw.Stream).BatchSize, 16)),
                        MaxAckPending:     intOrFirst([]string{"SIL_STREAM_MAX_ACK_PENDING"}, ptrOr(safe(raw.Stream).MaxAckPending, 256)),
                        ConnectTimeout:    durationOrFirst([]string{"SIL_STREAM_CONNECT_TIMEOUT"}, ptrOr(safe(raw.Stream).ConnectTimeout, "5s")),
                        Username:          envOrFirst([]string{"SIL_STREAM_USERNAME"}, ptrOr(safe(raw.Stream).Username, "")),
                        Password:          envOrFirst([]string{"SIL_STREAM_PASSWORD"}, ptrOr(safe(raw.Stream).Password, "")),
                        TLSEnabled:        boolOrFirst([]string{"SIL_STREAM_TLS_ENABLED"}, ptrOr(safe(raw.Stream).TLSEnabled, false)),
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
        if strings.TrimSpace(c.API.ListenAddr) == "" {
                return fmt.Errorf("api.listen_addr is required")
        }
        if c.API.ReadTimeout <= 0 {
                return fmt.Errorf("api.read_timeout must be positive")
        }
        if c.API.WriteTimeout <= 0 {
                return fmt.Errorf("api.write_timeout must be positive")
        }
        if c.API.IdleTimeout <= 0 {
                return fmt.Errorf("api.idle_timeout must be positive")
        }
        if !validLogLevel(c.Logging.Level) {
                return fmt.Errorf("unsupported logging.level %q", c.Logging.Level)
        }
        if !validLogFormat(c.Logging.Format) {
                return fmt.Errorf("unsupported logging.format %q", c.Logging.Format)
        }
        if c.Logging.AsyncQueueSize <= 0 {
                return fmt.Errorf("logging.async_queue_size must be positive")
        }
        if c.Logging.FlushTimeout <= 0 {
                return fmt.Errorf("logging.flush_timeout must be positive")
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
        if strings.TrimSpace(c.Sources.ConfigDir) == "" {
                return fmt.Errorf("sources.config_dir is required")
        }
        if len(normalizeStringList(c.Sources.Enabled)) == 0 {
                return fmt.Errorf("sources.enabled must contain at least one source")
        }
        if len(uniqueStrings(c.Sources.Enabled)) != len(normalizeStringList(c.Sources.Enabled)) {
                return fmt.Errorf("sources.enabled must be unique")
        }
        if strings.TrimSpace(c.Channels.ConfigDir) == "" {
                return fmt.Errorf("channels.config_dir is required")
        }
        if strings.TrimSpace(c.Channels.WebhookBasePath) == "" {
                return fmt.Errorf("channels.webhook_base_path is required")
        }
        if !strings.HasPrefix(c.Channels.WebhookBasePath, "/") {
                return fmt.Errorf("channels.webhook_base_path must start with '/'")
        }
        if c.Channels.RequestBodyLimitBytes <= 0 {
                return fmt.Errorf("channels.request_body_limit_bytes must be positive")
        }
        if len(uniqueStrings(c.Channels.Enabled)) != len(normalizeStringList(c.Channels.Enabled)) {
                return fmt.Errorf("channels.enabled must be unique")
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
                if !validStreamBackend(c.Stream.Backend) {
                        return fmt.Errorf("unsupported stream.backend %q", c.Stream.Backend)
                }
                if strings.TrimSpace(c.Stream.Addr) == "" {
                        return fmt.Errorf("stream.addr is required when stream backend is %q", c.Stream.Backend)
                }
                if strings.TrimSpace(c.Stream.StreamName) == "" {
                        return fmt.Errorf("stream.stream_name is required when stream backend is %q", c.Stream.Backend)
                }
                if !validStreamStorage(c.Stream.Storage) {
                        return fmt.Errorf("unsupported stream.storage %q", c.Stream.Storage)
                }
                for _, field := range []struct {
                        name  string
                        value string
                }{
                        {name: "stream.raw_subject", value: c.Stream.RawSubject},
                        {name: "stream.unified_subject", value: c.Stream.UnifiedSubject},
                        {name: "stream.index_subject", value: c.Stream.IndexSubject},
                        {name: "stream.deadletter_subject", value: c.Stream.DeadLetterSubject},
                        {name: "stream.consumer_prefix", value: c.Stream.ConsumerPrefix},
                } {
                        if strings.TrimSpace(field.value) == "" {
                                return fmt.Errorf("%s is required when stream backend is %q", field.name, c.Stream.Backend)
                        }
                }
                if c.Stream.AckWait <= 0 {
                        return fmt.Errorf("stream.ack_wait must be positive")
                }
                if c.Stream.MaxDeliveries <= 0 {
                        return fmt.Errorf("stream.max_deliveries must be positive")
                }
                if c.Stream.BatchSize <= 0 {
                        return fmt.Errorf("stream.batch_size must be positive")
                }
                if c.Stream.MaxAckPending <= 0 {
                        return fmt.Errorf("stream.max_ack_pending must be positive")
                }
                if c.Stream.ConnectTimeout <= 0 {
                        return fmt.Errorf("stream.connect_timeout must be positive")
                }
                if len(uniqueStrings(c.Stream.Subjects())) != len(c.Stream.Subjects()) {
                        return fmt.Errorf("stream subjects must be unique")
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

func validStreamBackend(backend string) bool {
        switch strings.ToLower(strings.TrimSpace(backend)) {
        case "jetstream":
                return true
        default:
                return false
        }
}

func validStreamStorage(storage string) bool {
        switch strings.ToLower(strings.TrimSpace(storage)) {
        case "file", "memory":
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

func durationOrFirst(keys []string, fallback string) time.Duration {
        for _, key := range keys {
                if value, ok := os.LookupEnv(key); ok {
                        value = strings.TrimSpace(value)
                        if value == "" {
                                continue
                        }
                        parsed, err := time.ParseDuration(value)
                        if err == nil {
                                return parsed
                        }
                }
        }

        parsed, err := time.ParseDuration(strings.TrimSpace(fallback))
        if err != nil {
                return 0
        }
        return parsed
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

func listOrFirst(keys []string, fallback []string) []string {
        for _, key := range keys {
                if value, ok := os.LookupEnv(key); ok {
                        parsed := normalizeStringList(strings.Split(value, ","))
                        if len(parsed) > 0 {
                                return parsed
                        }
                }
        }
        return normalizeStringList(fallback)
}

func sliceOrDefault(values, fallback []string) []string {
        if len(values) == 0 {
                return append([]string(nil), fallback...)
        }
        return append([]string(nil), values...)
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

func (cfg StreamConfig) Subjects() []string {
        return []string{
                cfg.RawSubject,
                cfg.UnifiedSubject,
                cfg.IndexSubject,
                cfg.DeadLetterSubject,
        }
}

func normalizeStringList(values []string) []string {
        normalized := make([]string, 0, len(values))
        for _, value := range values {
                value = strings.TrimSpace(value)
                if value == "" {
                        continue
                }
                normalized = append(normalized, value)
        }
        return normalized
}

func uniqueStrings(values []string) []string {
        values = normalizeStringList(values)
        seen := make(map[string]struct{}, len(values))
        var unique []string
        for _, value := range values {
                if _, ok := seen[value]; ok {
                        continue
                }
                seen[value] = struct{}{}
                unique = append(unique, value)
        }
        return unique
}
