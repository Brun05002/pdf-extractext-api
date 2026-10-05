package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_OK(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("MAX_FILE_SIZE_MB", "50")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://extraction:8081", cfg.ExtractionURL)
	assert.Equal(t, "http://persistence:8082", cfg.PersistenceURL)
	assert.Equal(t, int64(50), cfg.MaxFileSizeMB)
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, int64(20), cfg.MaxFileSizeMB, "debe aplicar default de 20MB")
	assert.Equal(t, ":8000", cfg.Port, "debe aplicar puerto default")
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "")
	t.Setenv("PERSISTENCE_URL", "")

	cfg, err := LoadConfig()
	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EXTRACTION_URL")
}

func TestLoadConfig_InvalidMaxFileSize(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("MAX_FILE_SIZE_MB", "no-es-un-numero")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_FILE_SIZE_MB")
}
