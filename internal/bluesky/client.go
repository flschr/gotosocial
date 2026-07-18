// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"

	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newATClient(state *state.State, host string) *atclient.APIClient {
	client := atclient.NewAPIClient(host)
	client.Client = protectedHTTPClient(state)
	return client
}

func protectedHTTPClient(state *state.State) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := state.HTTPClient.Do(request.WithContext(gtscontext.SetFastFail(request.Context())))
		if err != nil {
			return nil, err
		}
		return decodeGzipResponse(response)
	})}
}

// decodeGzipResponse handles ATProto services which return a gzip body without
// a usable Content-Encoding header. The shared HTTP client already handles
// correctly labelled compression, so the magic-byte check avoids double decode.
func decodeGzipResponse(response *http.Response) (*http.Response, error) {
	if response == nil || response.Body == nil {
		return response, nil
	}

	buffered := bufio.NewReader(response.Body)
	header, err := buffered.Peek(2)
	if err != nil && err != io.EOF {
		response.Body.Close()
		return nil, fmt.Errorf("inspect Bluesky response compression: %w", err)
	}
	if len(header) != 2 || header[0] != 0x1f || header[1] != 0x8b {
		response.Body = &bufferedReadCloser{Reader: buffered, Closer: response.Body}
		return response, nil
	}

	reader, err := gzip.NewReader(buffered)
	if err != nil {
		response.Body.Close()
		return nil, fmt.Errorf("decode Bluesky gzip response: %w", err)
	}
	response.Body = &gzipReadCloser{Reader: reader, gzip: reader, body: response.Body}
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-Length")
	response.ContentLength = -1
	response.Uncompressed = true
	return response, nil
}

type bufferedReadCloser struct {
	io.Reader
	io.Closer
}

type gzipReadCloser struct {
	io.Reader
	gzip *gzip.Reader
	body io.Closer
}

func (r *gzipReadCloser) Close() error {
	gzipErr := r.gzip.Close()
	bodyErr := r.body.Close()
	if gzipErr != nil {
		return gzipErr
	}
	return bodyErr
}
