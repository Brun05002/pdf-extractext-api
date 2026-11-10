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
		"SERVICE_NAME", "SERVICE_HEALTH_PATH", "PORT",
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
	t.Setenv("PORT", "")
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, int64(16), cfg.MaxFileSizeMB, "debe aplicar default de 16MB")
	assert.Equal(t, ":8000", cfg.Port, "debe aplicar puerto default")
	assert.Equal(t, 8000, cfg.ServicePort, "Consul debe usar el mismo puerto")
}

func TestLoadConfig_URLDefaults(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "")
	t.Setenv("PERSISTENCE_URL", "")
	t.Setenv("PERSISTENCE_HOST", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://traefik:8090", cfg.ExtractionURL)
	assert.Equal(t, "http://traefik:8090", cfg.PersistenceURL)
	assert.Equal(t, "persistence.pdf-extractext.localhost", cfg.PersistenceHost)
}

func TestLoadConfig_InvalidMaxFileSize(t *testing.T) {
	for _, raw := range []string{"no-es-un-numero", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("EXTRACTION_URL", "http://extraction:8081")
			t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
			t.Setenv("MAX_FILE_SIZE_MB", raw)

			_, err := LoadConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MAX_FILE_SIZE_MB")
		})
	}
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
	t.Setenv("PORT", "9000")
	t.Setenv("SERVICE_HEALTH_PATH", "/health")
	t.Setenv("CONSUL_HEALTH_CACHE_TTL", "5s")
	t.Setenv("CONSUL_HEALTH_FAIL_OPEN", "false")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://consul.local:8501", cfg.ConsulAddr)
	assert.Equal(t, "secreto", cfg.ConsulToken)
	assert.True(t, cfg.ConsulEnabled)
	assert.Equal(t, "api", cfg.ServiceName)
	assert.Equal(t, ":9000", cfg.Port)
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

func TestLoadConfig_InvalidPort(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")
	t.Setenv("PORT", "no-es-puerto")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT")
}

func TestLoadConfig_PortWithColon(t *testing.T) {
	clearConsulEnv(t)
	t.Setenv("PORT", ":9001")
	t.Setenv("EXTRACTION_URL", "http://extraction:8081")
	t.Setenv("PERSISTENCE_URL", "http://persistence:8082")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, ":9001", cfg.Port)
	assert.Equal(t, 9001, cfg.ServicePort)
}

func TestApplyKV_EnvWinsOverKV(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "http://env-extraction:1")
	t.Setenv("EXTRACTION_HOST", "env.extraction.host")
	t.Setenv("PERSISTENCE_URL", "http://env-persistence:2")
	t.Setenv("PERSISTENCE_HOST", "env.persistence.host")
	t.Setenv("MAX_FILE_SIZE_MB", "20")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	err = ApplyKV(cfg, map[string]string{
		"EXTRACTION_URL":   "http://kv-extraction:3",
		"EXTRACTION_HOST":  "kv.extraction.host",
		"PERSISTENCE_URL":  "http://kv-persistence:4",
		"PERSISTENCE_HOST": "kv.persistence.host",
		"MAX_FILE_SIZE_MB": "99",
	})
	require.NoError(t, err)

	assert.Equal(t, "http://env-extraction:1", cfg.ExtractionURL)
	assert.Equal(t, "env.extraction.host", cfg.ExtractionHost)
	assert.Equal(t, "http://env-persistence:2", cfg.PersistenceURL)
	assert.Equal(t, "env.persistence.host", cfg.PersistenceHost)
	assert.Equal(t, int64(20), cfg.MaxFileSizeMB)
}

func TestApplyKV_KVWinsOverDefaults(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "")
	t.Setenv("EXTRACTION_HOST", "")
	t.Setenv("PERSISTENCE_URL", "")
	t.Setenv("PERSISTENCE_HOST", "")
	t.Setenv("MAX_FILE_SIZE_MB", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	err = ApplyKV(cfg, map[string]string{
		"EXTRACTION_URL":   "http://kv-extraction:3",
		"EXTRACTION_HOST":  "kv.extraction.host",
		"PERSISTENCE_URL":  "http://kv-persistence:4",
		"PERSISTENCE_HOST": "kv.persistence.host",
		"MAX_FILE_SIZE_MB": "64",
	})
	require.NoError(t, err)

	assert.Equal(t, "http://kv-extraction:3", cfg.ExtractionURL)
	assert.Equal(t, "kv.extraction.host", cfg.ExtractionHost)
	assert.Equal(t, "http://kv-persistence:4", cfg.PersistenceURL)
	assert.Equal(t, "kv.persistence.host", cfg.PersistenceHost)
	assert.Equal(t, int64(64), cfg.MaxFileSizeMB)
}

func TestApplyKV_EmptyValuesKeepDefaults(t *testing.T) {
	t.Setenv("EXTRACTION_URL", "")
	t.Setenv("EXTRACTION_HOST", "")
	t.Setenv("PERSISTENCE_URL", "")
	t.Setenv("PERSISTENCE_HOST", "")
	t.Setenv("MAX_FILE_SIZE_MB", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	err = ApplyKV(cfg, map[string]string{"EXTRACTION_URL": "", "MAX_FILE_SIZE_MB": ""})
	require.NoError(t, err)

	assert.Equal(t, "http://traefik:8090", cfg.ExtractionURL)
	assert.Equal(t, int64(16), cfg.MaxFileSizeMB)
}

func TestApplyKV_IgnoresUnknownKeysAndSecrets(t *testing.T) {
	t.Setenv("CONSUL_HTTP_TOKEN", "")
	t.Setenv("EXTRACTION_URL", "")
	t.Setenv("PERSISTENCE_URL", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	err = ApplyKV(cfg, map[string]string{
		"MONGO_URI":         "mongodb://secreto",
		"CONSUL_HTTP_TOKEN": "token-filtrado",
		"CLAVE_DESCONOCIDA": "ignorada",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsulToken)
}

func TestApplyKV_InvalidMaxFileSize(t *testing.T) {
	for _, raw := range []string{"no-es-numero", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("EXTRACTION_URL", "")
			t.Setenv("EXTRACTION_HOST", "")
			t.Setenv("PERSISTENCE_URL", "")
			t.Setenv("PERSISTENCE_HOST", "")
			t.Setenv("MAX_FILE_SIZE_MB", "")

			cfg, err := LoadConfig()
			require.NoError(t, err)
			before := *cfg

			err = ApplyKV(cfg, map[string]string{
				"EXTRACTION_URL":   "http://kv-extraction:3",
				"EXTRACTION_HOST":  "kv.extraction.host",
				"PERSISTENCE_URL":  "http://kv-persistence:4",
				"PERSISTENCE_HOST": "kv.persistence.host",
				"MAX_FILE_SIZE_MB": raw,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MAX_FILE_SIZE_MB")
			assert.Equal(t, before, *cfg, "un KV inválido no debe mutar parcialmente la configuración")
		})
	}
}
