package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertProblemBody verifica el contrato RFC 9457: campos exactos
// {type, title, status, detail, code} con el URN y status esperados.
func assertProblemBody(t *testing.T, body string, status int, code, urnSlug, detail string) {
	t.Helper()
	var problem map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &problem))
	assert.Len(t, problem, 5, "el problem debe tener exactamente 5 campos: %s", body)
	assert.Equal(t, "urn:pdf-extractext:"+urnSlug, problem["type"])
	assert.Equal(t, float64(status), problem["status"])
	assert.Equal(t, code, problem["code"])
	assert.Equal(t, detail, problem["detail"])
	assert.NotEmpty(t, problem["title"])
}
