package core

// FASE GREEN: implementación mínima para pasar los tests.

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port           string
	ExtractionURL  string
	PersistenceURL string
	MaxFileSizeMB  int64
}

func LoadConfig() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":3000"
	}

	extractionURL := os.Getenv("EXTRACTION_URL")
	if extractionURL == "" {
		return nil, fmt.Errorf("EXTRACTION_URL es obligatoria")
	}

	persistenceURL := os.Getenv("PERSISTENCE_URL")
	if persistenceURL == "" {
		return nil, fmt.Errorf("PERSISTENCE_URL es obligatoria")
	}

	maxFileSize, err := loadMaxFileSize()
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:           port,
		ExtractionURL:  extractionURL,
		PersistenceURL: persistenceURL,
		MaxFileSizeMB:  maxFileSize,
	}, nil
}

func loadMaxFileSize() (int64, error) {
	raw := os.Getenv("MAX_FILE_SIZE_MB")
	if raw == "" {
		return 20, nil
	}
	size, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("MAX_FILE_SIZE_MB inválido: %w", err)
	}
	return size, nil
}
