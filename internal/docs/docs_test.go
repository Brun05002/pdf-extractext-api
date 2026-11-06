package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", name))
	require.NoError(t, err)
	return string(data)
}

func TestEnvExample_DocumentsAllVariables(t *testing.T) {
	env := readRepoFile(t, ".env.example")

	for _, key := range []string{
		"PORT", "EXTRACTION_URL", "EXTRACTION_HOST",
		"PERSISTENCE_URL", "PERSISTENCE_HOST", "MAX_FILE_SIZE_MB",
		"CONSUL_ENABLED", "CONSUL_HTTP_ADDR", "CONSUL_HTTP_TOKEN",
		"SERVICE_NAME", "SERVICE_PORT", "SERVICE_HEALTH_PATH",
		"CONSUL_HEALTH_CACHE_TTL", "CONSUL_HEALTH_FAIL_OPEN",
	} {
		assert.Contains(t, env, key+"=", "falta %s en .env.example", key)
	}
	assert.NotContains(t, env, "CONSUL_HTTP_TOKEN=secreto")
}

func TestReadme_DocumentsConsulIntegration(t *testing.T) {
	readme := readRepoFile(t, "README.md")

	for _, key := range []string{
		"CONSUL_ENABLED", "CONSUL_HTTP_ADDR", "CONSUL_HTTP_TOKEN",
		"CONSUL_HEALTH_CACHE_TTL", "CONSUL_HEALTH_FAIL_OPEN",
		"PERSISTENCE_HOST", "SERVICE_HEALTH_PATH",
	} {
		assert.Contains(t, readme, key, "README debe documentar %s", key)
	}

	lower := strings.ToLower(readme)
	for _, term := range []string{"health gate", "registro", "kv", "sigterm", "dependency_unavailable"} {
		assert.Contains(t, lower, term, "README debe explicar %q", term)
	}
}

func TestChangelog_DocumentsConsulIntegration(t *testing.T) {
	changelog := readRepoFile(t, "CHANGELOG.md")

	assert.Contains(t, changelog, "Consul")
	assert.Contains(t, changelog, "Traefik")
}

func TestCI_RunsVetLintRaceAndCoverage(t *testing.T) {
	ci := readRepoFile(t, ".github/workflows/ci.yml")

	for _, step := range []string{"go vet ./...", "golangci-lint", "go test -race -cover ./..."} {
		assert.Contains(t, ci, step)
	}
}
