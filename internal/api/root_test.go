package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot_HealthEnriched(t *testing.T) {
	app := fiber.New()
	RegisterRoot(app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "pdf-extractext-api", body["service"])
	assert.Equal(t, "UP", body["status"])

	ts, err := time.Parse(time.RFC3339, body["time"])
	require.NoError(t, err, "time debe ser RFC3339 válido")
	assert.WithinDuration(t, time.Now().UTC(), ts, 10*time.Second)
}
