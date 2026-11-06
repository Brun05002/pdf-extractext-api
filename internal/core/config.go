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

// defaultExtractionHost es el Host que el router interno de Traefik (:8090) usa para enrutar a Extraction.
const defaultExtractionHost = "extraction.pdf-extractext.localhost"

func LoadConfig() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8000"
	}

	extractionURL := os.Getenv("EXTRACTION_URL")
	if extractionURL == "" {
		return nil, fmt.Errorf("EXTRACTION_URL es obligatoria")
	}

	extractionHost := os.Getenv("EXTRACTION_HOST")
	if extractionHost == "" {
		extractionHost = defaultExtractionHost
	}

	persistenceURL := os.Getenv("PERSISTENCE_URL")
	if persistenceURL == "" {
		return nil, fmt.Errorf("PERSISTENCE_URL es obligatoria")
	}

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

func loadMaxFileSize() (int64, error) {
	raw := os.Getenv("MAX_FILE_SIZE_MB")
	if raw == "" {
		return 16, nil
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
