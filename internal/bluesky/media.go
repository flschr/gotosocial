// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/media"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
	_ "golang.org/x/image/webp"
)

const (
	maxSourceImageBytes  = 50 << 20
	maxSourceImagePixels = 40_000_000
)

func uploadStoredBlob(ctx context.Context, state *state.State, client *atclient.APIClient, path, contentType string) (any, error) {
	stream, err := state.Storage.GetStream(ctx, path)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := readBlueskyImage(stream)
	if err != nil {
		return nil, err
	}
	data, contentType, err = prepareBlueskyImage(data)
	if err != nil {
		return nil, err
	}
	return uploadBlob(ctx, client, bytes.NewReader(data), contentType)
}

func uploadRemoteImage(ctx context.Context, state *state.State, client *atclient.APIClient, imageURL string) (any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := state.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("preview image returned %s", response.Status)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("preview is not an image")
	}
	data, err := readBlueskyImage(response.Body)
	if err != nil {
		return nil, err
	}
	data, contentType, err = prepareBlueskyImage(data)
	if err != nil {
		return nil, err
	}
	return uploadBlob(ctx, client, bytes.NewReader(data), contentType)
}

// prepareBlueskyImage re-encodes an image to remove EXIF and other embedded
// metadata, then progressively compresses/resizes it to Bluesky's blob limit.
func prepareBlueskyImage(data []byte) ([]byte, string, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("inspect image for Bluesky: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxSourceImagePixels {
		return nil, "", fmt.Errorf("image dimensions exceed Bluesky processing limit")
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image for Bluesky: %w", err)
	}
	for source.Bounds().Dx() > 2000 || source.Bounds().Dy() > 2000 {
		source = resizeImage(source, 0.85)
	}
	for quality := 92; ; quality -= 8 {
		var encoded bytes.Buffer
		// JPEG has broad PDS support. Drawing onto an opaque white background
		// produces predictable results for transparent PNG/WebP sources.
		opaque := image.NewRGBA(image.Rect(0, 0, source.Bounds().Dx(), source.Bounds().Dy()))
		draw.Draw(opaque, opaque.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(opaque, opaque.Bounds(), source, source.Bounds().Min, draw.Over)
		if err := jpeg.Encode(&encoded, opaque, &jpeg.Options{Quality: max(quality, 60)}); err != nil {
			return nil, "", err
		}
		if encoded.Len() <= maxBlobBytes {
			return encoded.Bytes(), "image/jpeg", nil
		}
		if quality <= 60 {
			source = resizeImage(source, 0.8)
			quality = 92
			if source.Bounds().Dx() < 320 || source.Bounds().Dy() < 320 {
				return nil, "", fmt.Errorf("image cannot be compressed below Bluesky blob limit")
			}
		}
	}
}

func readBlueskyImage(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxSourceImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSourceImageBytes {
		return nil, fmt.Errorf("image exceeds Bluesky processing size limit")
	}
	return data, nil
}

func resizeImage(source image.Image, scale float64) image.Image {
	width := max(1, int(float64(source.Bounds().Dx())*scale))
	height := max(1, int(float64(source.Bounds().Dy())*scale))
	return media.ResizeDownLinear(source, width, height)
}

func uploadBlob(ctx context.Context, client *atclient.APIClient, reader io.Reader, contentType string) (any, error) {
	endpoint, _ := syntax.ParseNSID("com.atproto.repo.uploadBlob")
	request := atclient.NewAPIRequest(http.MethodPost, endpoint, reader)
	request.Headers.Set("Content-Type", contentType)
	var response uploadBlobResponse
	result, err := client.Do(ctx, request)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return nil, fmt.Errorf("blob upload returned %s", result.Status)
	}
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Blob, nil
}
