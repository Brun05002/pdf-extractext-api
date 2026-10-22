package core

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port           string
	ExtractionURL  string
	ExtractionHost string
	PersistenceURL string
	MaxFileSizeMB  int64
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

	return &Config{
		Port:           port,
		ExtractionURL:  extractionURL,
		ExtractionHost: extractionHost,
		PersistenceURL: persistenceURL,
		MaxFileSizeMB:  maxFileSize,
	}, nil
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
