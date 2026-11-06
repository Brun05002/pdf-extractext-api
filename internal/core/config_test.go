package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearConsulEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CONSUL_HTTP_ADDR", "CONSUL_HTTP_TOKEN", "CONSUL_ENABLED",
		"SERVICE_NAME", "SERVICE_PORT", "SERVICE_HEALTH_PATH",
		"CONSUL_HEALTH_CACHE_TTL", "CONSUL_HEALTH_FAIL_OPEN",
	} {
		t.Setenv(key, "")
	}
}

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
	assert.Equal(t, int64(16), cfg.MaxFileSizeMB, "debe aplicar default de 16MB")
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

func TestLoadConfig_ConsulDefaults(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://consul:8500", cfg.ConsulAddr)
	assert.Empty(t, cfg.ConsulToken)
	assert.False(t, cfg.ConsulEnabled)
	assert.Equal(t, "pdf-extractext-api", cfg.ServiceName)
	assert.Equal(t, 8000, cfg.ServicePort)
	assert.Equal(t, "/", cfg.ServiceHealthPath)
	assert.Equal(t, 3*time.Second, cfg.ConsulHealthCacheTTL)
	assert.True(t, cfg.ConsulHealthFailOpen)
}

func TestLoadConfig_ConsulOverrides(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("CONSUL_HTTP_ADDR", "http://consul.local:8501")
	t.Setenv("CONSUL_HTTP_TOKEN", "secreto")
	t.Setenv("CONSUL_ENABLED", "true")
	t.Setenv("SERVICE_NAME", "api")
	t.Setenv("SERVICE_PORT", "9000")
	t.Setenv("SERVICE_HEALTH_PATH", "/health")
	t.Setenv("CONSUL_HEALTH_CACHE_TTL", "5s")
	t.Setenv("CONSUL_HEALTH_FAIL_OPEN", "false")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://consul.local:8501", cfg.ConsulAddr)
	assert.Equal(t, "secreto", cfg.ConsulToken)
	assert.True(t, cfg.ConsulEnabled)
	assert.Equal(t, "api", cfg.ServiceName)
	assert.Equal(t, 9000, cfg.ServicePort)
	assert.Equal(t, "/health", cfg.ServiceHealthPath)
	assert.Equal(t, 5*time.Second, cfg.ConsulHealthCacheTTL)
	assert.False(t, cfg.ConsulHealthFailOpen)
}

func TestLoadConfig_InvalidConsulTTL(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("CONSUL_HEALTH_CACHE_TTL", "no-es-duracion")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONSUL_HEALTH_CACHE_TTL")
}

func TestLoadConfig_InvalidServicePort(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("SERVICE_PORT", "no-es-puerto")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SERVICE_PORT")
}
