// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeGzipResponseWithoutEncodingHeader(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"notifications":[]}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	response, err := decodeGzipResponse(&http.Response{
		Body:          io.NopCloser(bytes.NewReader(compressed.Bytes())),
		Header:        http.Header{"Content-Length": []string{"42"}},
		ContentLength: 42,
	})
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"notifications":[]}`, string(body))
	require.Equal(t, int64(-1), response.ContentLength)
	require.True(t, response.Uncompressed)
	require.Empty(t, response.Header.Get("Content-Length"))
}

func TestDecodeGzipResponseLeavesPlainBodyUntouched(t *testing.T) {
	response, err := decodeGzipResponse(&http.Response{
		Body:   io.NopCloser(bytes.NewBufferString(`{"notifications":[]}`)),
		Header: make(http.Header),
	})
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"notifications":[]}`, string(body))
	require.False(t, response.Uncompressed)
}
