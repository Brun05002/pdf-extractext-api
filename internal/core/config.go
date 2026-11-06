package core

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	ExtractionURL   string
	ExtractionHost  string
	PersistenceURL  string
	PersistenceHost string
	MaxFileSizeMB   int64

	ConsulAddr           string
	ConsulToken          string
	ConsulEnabled        bool
	ServiceName          string
	ServicePort          int
	ServiceHealthPath    string
	ConsulHealthCacheTTL time.Duration
	ConsulHealthFailOpen bool
}

// Defaults de las URLs y Hosts que usa el router interno de Traefik (:8090).
const (
	defaultExtractionURL   = "http://traefik:8090"
	defaultExtractionHost  = "extraction.pdf-extractext.localhost"
	defaultPersistenceURL  = "http://traefik:8090"
	defaultPersistenceHost = "persistence.pdf-extractext.localhost"
	defaultMaxFileSizeMB   = 16
)

func LoadConfig() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8000"
	}

	extractionURL := envOrDefault("EXTRACTION_URL", defaultExtractionURL)
	extractionHost := envOrDefault("EXTRACTION_HOST", defaultExtractionHost)
	persistenceURL := envOrDefault("PERSISTENCE_URL", defaultPersistenceURL)
	persistenceHost := envOrDefault("PERSISTENCE_HOST", defaultPersistenceHost)

	maxFileSize, err := loadMaxFileSize()
	if err != nil {
		return nil, err
	}

	consulEnabled, err := loadBool("CONSUL_ENABLED", false)
	if err != nil {
		return nil, err
	}

	servicePort, err := loadInt("SERVICE_PORT", 8000)
	if err != nil {
		return nil, err
	}

	healthTTL, err := loadDuration("CONSUL_HEALTH_CACHE_TTL", 3*time.Second)
	if err != nil {
		return nil, err
	}

	failOpen, err := loadBool("CONSUL_HEALTH_FAIL_OPEN", true)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:                 port,
		ExtractionURL:        extractionURL,
		ExtractionHost:       extractionHost,
		PersistenceURL:       persistenceURL,
		PersistenceHost:      persistenceHost,
		MaxFileSizeMB:        maxFileSize,
		ConsulAddr:           envOrDefault("CONSUL_HTTP_ADDR", "http://consul:8500"),
		ConsulToken:          os.Getenv("CONSUL_HTTP_TOKEN"),
		ConsulEnabled:        consulEnabled,
		ServiceName:          envOrDefault("SERVICE_NAME", "pdf-extractext-api"),
		ServicePort:          servicePort,
		ServiceHealthPath:    envOrDefault("SERVICE_HEALTH_PATH", "/"),
		ConsulHealthCacheTTL: healthTTL,
		ConsulHealthFailOpen: failOpen,
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// ApplyKV aplica solo claves conocidas y nunca pisa una variable de entorno explícita.
func ApplyKV(cfg *Config, values map[string]string) error {
	applyKVString(&cfg.ExtractionURL, "EXTRACTION_URL", values)
	applyKVString(&cfg.ExtractionHost, "EXTRACTION_HOST", values)
	applyKVString(&cfg.PersistenceURL, "PERSISTENCE_URL", values)
	applyKVString(&cfg.PersistenceHost, "PERSISTENCE_HOST", values)

	raw := values["MAX_FILE_SIZE_MB"]
	if raw == "" || os.Getenv("MAX_FILE_SIZE_MB") != "" {
		return nil
	}
	size, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("MAX_FILE_SIZE_MB inválido en KV: %w", err)
	}
	cfg.MaxFileSizeMB = size
	return nil
}

func applyKVString(target *string, envKey string, values map[string]string) {
	value := values[envKey]
	if value == "" || os.Getenv(envKey) != "" {
		return
	}
	*target = value
}

func loadMaxFileSize() (int64, error) {
	raw := os.Getenv("MAX_FILE_SIZE_MB")
	if raw == "" {
		return defaultMaxFileSizeMB, nil
	}
	size, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("MAX_FILE_SIZE_MB inválido: %w", err)
	}
	return size, nil
}

func loadInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %w", key, err)
	}
	return value, nil
}

func loadDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s inválido: %w", key, err)
	}
	return value, nil
}

func loadBool(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s inválido: %w", key, err)
	}
	return value, nil
}
