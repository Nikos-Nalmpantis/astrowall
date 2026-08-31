package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	credentialService       = "astrowall"
	credentialAccount       = "NASA_API_KEY"
	credentialLookupTimeout = 2 * time.Second
)

type credentialLookupResult struct {
	apiKey string
	err    error
}

type apiKeySource string

const (
	apiKeySourceFlag       apiKeySource = "command line"
	apiKeySourceEnv        apiKeySource = "environment"
	apiKeySourceCredential apiKeySource = "saved credential"
	apiKeySourceDemo       apiKeySource = "DEMO_KEY"
)

var (
	keyringGet    = keyring.Get
	keyringSet    = keyring.Set
	keyringDelete = keyring.Delete
	validateKey   = validateAPIKey
)

func resolveAPIKeyWithSource(flagValue string, getenv func(string) string) (string, apiKeySource, error) {
	if apiKey, source, ok := resolveExplicitAPIKey(flagValue, getenv); ok {
		return apiKey, source, nil
	}
	value, err := keyringGet(credentialService, credentialAccount)
	if err == nil && value != "" {
		return value, apiKeySourceCredential, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "DEMO_KEY", apiKeySourceDemo, fmt.Errorf("reading saved NASA API key: %w", err)
	}
	return "DEMO_KEY", apiKeySourceDemo, nil
}

func resolveExplicitAPIKey(flagValue string, getenv func(string) string) (string, apiKeySource, bool) {
	if flagValue != "" {
		return flagValue, apiKeySourceFlag, true
	}
	if value := getenv("NASA_API_KEY"); value != "" {
		return value, apiKeySourceEnv, true
	}
	return "", "", false
}

func lookupSavedAPIKey(timeout time.Duration) (string, error) {
	result := make(chan credentialLookupResult, 1)
	get := keyringGet
	go func() {
		apiKey, err := get(credentialService, credentialAccount)
		result <- credentialLookupResult{apiKey: apiKey, err: err}
	}()
	select {
	case lookup := <-result:
		return lookup.apiKey, lookup.err
	case <-time.After(timeout):
		return "", fmt.Errorf("OS credential store lookup timed out")
	}
}

func credentialNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

func saveAPIKey(apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("NASA API key cannot be empty")
	}
	if err := validateKey(apiKey); err != nil {
		return err
	}
	if err := keyringSet(credentialService, credentialAccount, apiKey); err != nil {
		return fmt.Errorf("saving NASA API key to OS credential store: %w", err)
	}
	return nil
}

func removeSavedAPIKey() error {
	if err := keyringDelete(credentialService, credentialAccount); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("removing saved NASA API key: %w", err)
	}
	return nil
}

func validateAPIKey(apiKey string) error {
	response, err := httpGet(buildAPODURL(apiKey, false, ""))
	if err != nil {
		return fmt.Errorf("NASA API key validation request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("NASA API key validation returned HTTP %d", response.StatusCode)
	}
	return nil
}

func readAPIKey(in *os.File, out io.Writer) (string, error) {
	if !term.IsTerminal(int(in.Fd())) {
		return "", fmt.Errorf("interactive terminal input is required")
	}
	fmt.Fprint(out, "NASA API key: ")
	value, err := term.ReadPassword(int(in.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("reading NASA API key: %w", err)
	}
	return strings.TrimSpace(string(value)), nil
}
