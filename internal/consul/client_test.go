package consul

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "token-secreto"

func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, testToken)
}

func TestClient_Register_PUTWithTokenAndJSON(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/agent/service/register", r.URL.Path)
		assert.Equal(t, testToken, r.Header.Get("X-Consul-Token"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "api-host", payload["ID"])
		w.WriteHeader(http.StatusOK)
	}))

	err := client.Register(t.Context(), map[string]any{"ID": "api-host"})
	require.NoError(t, err)
}

func TestClient_Deregister_PUTWithEscapedID(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/agent/service/deregister/api-host", r.URL.Path)
		assert.Equal(t, testToken, r.Header.Get("X-Consul-Token"))
		w.WriteHeader(http.StatusOK)
	}))

	require.NoError(t, client.Deregister(t.Context(), "api-host"))
}

func TestClient_HealthPassing(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/health/service/extraction", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("passing"))
		assert.Equal(t, testToken, r.Header.Get("X-Consul-Token"))

		if r.URL.Path == "/v1/health/service/extraction" {
			_, _ = w.Write([]byte(`[{"Service":{"ID":"extraction-1"}}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))

	healthy, err := client.HealthPassing(t.Context(), "extraction")
	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestClient_HealthPassing_EmptyIsFalse(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))

	healthy, err := client.HealthPassing(t.Context(), "persistence")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestClient_KVList_DecodesBase64AndStripsPrefix(t *testing.T) {
	encode := func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) }

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/kv/pdf-extractext/config/api", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("recurse"))
		assert.Equal(t, testToken, r.Header.Get("X-Consul-Token"))

		_, _ = w.Write([]byte(`[
			{"Key":"pdf-extractext/config/api/EXTRACTION_URL","Value":"` + encode("http://traefik:8090") + `"},
			{"Key":"pdf-extractext/config/api/MAX_FILE_SIZE_MB","Value":"` + encode("32") + `"},
			{"Key":"pdf-extractext/config/api/SIN_VALOR","Value":null}
		]`))
	}))

	values, err := client.KVList(t.Context(), "pdf-extractext/config/api/")
	require.NoError(t, err)
	assert.Equal(t, "http://traefik:8090", values["EXTRACTION_URL"])
	assert.Equal(t, "32", values["MAX_FILE_SIZE_MB"])
	assert.Equal(t, "", values["SIN_VALOR"])
}

func TestClient_KVList_InvalidBase64ReturnsError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Key":"pdf-extractext/config/api/X","Value":"no-es-base64!!"}]`))
	}))

	_, err := client.KVList(t.Context(), "pdf-extractext/config/api/")
	require.Error(t, err)
}

func TestClient_HTTPError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("ACL denied"))
	}))

	err := client.Register(t.Context(), map[string]any{"ID": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestClient_Timeout(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	client.HTTPClient.Timeout = 50 * time.Millisecond

	err := client.Register(t.Context(), map[string]any{"ID": "x"})
	require.Error(t, err)
}

func TestClient_NoToken_OmitsHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-Consul-Token"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	require.NoError(t, client.Register(t.Context(), map[string]any{"ID": "x"}))
}

func TestClient_TrimsTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.False(t, strings.HasPrefix(r.URL.Path, "//"), "no debe duplicar la barra")
		assert.Equal(t, "/v1/agent/service/register", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL+"/", testToken)
	require.NoError(t, client.Register(t.Context(), map[string]any{"ID": "x"}))
}

func TestClient_NormalizesAddrWithoutScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/agent/service/register", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hostPort := strings.TrimPrefix(server.URL, "http://")
	client := NewClient(hostPort, testToken)
	assert.Equal(t, server.URL, client.BaseURL)
	require.NoError(t, client.Register(t.Context(), map[string]any{"ID": "x"}))
}
