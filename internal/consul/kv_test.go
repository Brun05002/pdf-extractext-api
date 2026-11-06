package consul

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configPrefix = "pdf-extractext/config/api/"

func TestLoadConfigKV_UsesConfigPrefix(t *testing.T) {
	source := &fakeKVSource{values: map[string]string{"EXTRACTION_URL": "http://traefik:8090"}}

	values, err := LoadConfigKV(t.Context(), source)
	require.NoError(t, err)
	assert.Equal(t, configPrefix, source.gotPrefix)
	assert.Equal(t, "http://traefik:8090", values["EXTRACTION_URL"])
}

func TestLoadConfigKV_PropagatesNetworkError(t *testing.T) {
	source := &fakeKVSource{err: errors.New("connection refused")}

	_, err := LoadConfigKV(t.Context(), source)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestLoadConfigKV_DecodesBase64FromConsul(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/kv/pdf-extractext/config/api", r.URL.Path)
		value := base64.StdEncoding.EncodeToString([]byte("http://traefik:8090"))
		_, _ = w.Write([]byte(`[{"Key":"pdf-extractext/config/api/EXTRACTION_URL","Value":"` + value + `"}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")

	values, err := LoadConfigKV(t.Context(), client)
	require.NoError(t, err)
	assert.Equal(t, "http://traefik:8090", values["EXTRACTION_URL"])
}

type fakeKVSource struct {
	values    map[string]string
	err       error
	gotPrefix string
}

func (f *fakeKVSource) KVList(ctx context.Context, prefix string) (map[string]string, error) {
	f.gotPrefix = prefix
	return f.values, f.err
}
