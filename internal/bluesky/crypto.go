// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const encryptionKeySize = 32

type Crypter struct {
	aead cipher.AEAD
}

func NewCrypter(encodedKey string) (*Crypter, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode Bluesky connection encryption key: %w", err)
	}
	if len(key) != encryptionKeySize {
		return nil, fmt.Errorf("Bluesky connection encryption key must decode to %d bytes", encryptionKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create Bluesky connection cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create Bluesky connection AEAD: %w", err)
	}
	return &Crypter{aead: aead}, nil
}

func (c *Crypter) Encrypt(plaintext, associatedData []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate Bluesky connection nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, associatedData), nil
}

func (c *Crypter) Decrypt(ciphertext, associatedData []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, fmt.Errorf("Bluesky connection ciphertext is too short")
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext[c.aead.NonceSize():], associatedData)
	if err != nil {
		return nil, fmt.Errorf("decrypt Bluesky connection data: %w", err)
	}
	return plaintext, nil
}
