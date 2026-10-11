package main

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// Tokens live in the OS keyring: macOS Keychain (via the security CLI), Linux Secret Service,
// Windows Credential Manager. Plaintext storage in the config file is opt-in only.
const (
	keyringService = "mevspace-cli"
	secretBackend  = "OS keyring"
)

// storeToken saves the token and returns where it went. It never silently falls back to plaintext.
func storeToken(c *Context, token string, allowPlaintext bool) (string, error) {
	if allowPlaintext {
		c.Token = token
		return "config file (0600, plaintext)", nil
	}
	if err := keyring.Set(keyringService, c.Name, token); err != nil {
		return "", fmt.Errorf("storing token in %s: %w\n\nUse --insecure-plaintext to store it in the config file instead, or set MEVSPACE_API_TOKEN", secretBackend, err)
	}
	c.Token = ""
	return secretBackend + " (service \"" + keyringService + "\")", nil
}

func loadToken(c *Context) (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	return keyring.Get(keyringService, c.Name)
}

func deleteToken(c *Context) error {
	if c.Token != "" {
		return nil
	}
	err := keyring.Delete(keyringService, c.Name)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
