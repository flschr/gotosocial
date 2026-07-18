// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrypter(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, encryptionKeySize))
	crypter, err := NewCrypter(key)
	require.NoError(t, err)

	associatedData := []byte("account/session")
	ciphertext, err := crypter.Encrypt([]byte("secret"), associatedData)
	require.NoError(t, err)
	require.NotContains(t, string(ciphertext), "secret")

	plaintext, err := crypter.Decrypt(ciphertext, associatedData)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), plaintext)

	_, err = crypter.Decrypt(ciphertext, []byte("other/session"))
	require.Error(t, err)
}

func TestCrypterRejectsInvalidKey(t *testing.T) {
	_, err := NewCrypter(base64.StdEncoding.EncodeToString([]byte("short")))
	require.Error(t, err)
}
