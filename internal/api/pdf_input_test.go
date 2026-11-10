package api

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyLimitReader_AllowsExactLimit(t *testing.T) {
	reader := &bodyLimitReader{reader: strings.NewReader("12345"), remaining: 5}

	body, err := io.ReadAll(reader)

	require.NoError(t, err)
	assert.Equal(t, "12345", string(body))
	assert.False(t, reader.exceeded)
}

func TestBodyLimitReader_StopsOnExcess(t *testing.T) {
	reader := &bodyLimitReader{reader: strings.NewReader("123456"), remaining: 5}

	body, err := io.ReadAll(reader)

	assert.ErrorIs(t, err, errBodyTooLarge)
	assert.Equal(t, "123456", string(body), "el exceso se detecta al leer el byte extra")
	assert.True(t, reader.exceeded)
}

func TestBodyLimitReader_ZeroLengthBuffer(t *testing.T) {
	reader := &bodyLimitReader{reader: strings.NewReader("data"), remaining: 4}

	n, err := reader.Read(nil)

	require.NoError(t, err)
	assert.Zero(t, n)
	assert.False(t, reader.exceeded)
}

func TestBodyLimitReader_RejectsNegativeLimit(t *testing.T) {
	reader := &bodyLimitReader{reader: strings.NewReader("data"), remaining: -1}

	_, err := reader.Read(make([]byte, 1))

	assert.True(t, errors.Is(err, errBodyTooLarge))
	assert.True(t, reader.exceeded)
}
