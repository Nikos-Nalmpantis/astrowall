package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func withCredentialStubs(t *testing.T) {
	t.Helper()
	originalGet := keyringGet
	originalSet := keyringSet
	originalDelete := keyringDelete
	originalValidate := validateKey
	t.Cleanup(func() {
		keyringGet = originalGet
		keyringSet = originalSet
		keyringDelete = originalDelete
		validateKey = originalValidate
	})
}

func TestResolveAPIKeyWithSourcePrecedence(t *testing.T) {
	withCredentialStubs(t)
	keyringGet = func(service, account string) (string, error) {
		return "SAVED", nil
	}

	tests := []struct {
		name   string
		flag   string
		env    string
		want   string
		source apiKeySource
	}{
		{name: "flag", flag: "FLAG", env: "ENV", want: "FLAG", source: apiKeySourceFlag},
		{name: "environment", env: "ENV", want: "ENV", source: apiKeySourceEnv},
		{name: "credential", want: "SAVED", source: apiKeySourceCredential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, source, err := resolveAPIKeyWithSource(tt.flag, func(string) string { return tt.env })
			if err != nil || key != tt.want || source != tt.source {
				t.Fatalf("resolveAPIKeyWithSource() = %q, %q, %v", key, source, err)
			}
		})
	}
}

func TestResolveAPIKeyWithSourceFallsBackToDemo(t *testing.T) {
	withCredentialStubs(t)
	keyringGet = func(service, account string) (string, error) {
		return "", keyring.ErrNotFound
	}
	key, source, err := resolveAPIKeyWithSource("", func(string) string { return "" })
	if err != nil || key != "DEMO_KEY" || source != apiKeySourceDemo {
		t.Fatalf("resolveAPIKeyWithSource() = %q, %q, %v", key, source, err)
	}
}

func TestSaveAPIKeyValidatesBeforeKeyringWrite(t *testing.T) {
	withCredentialStubs(t)
	var validated, saved string
	validateKey = func(key string) error {
		validated = key
		return nil
	}
	keyringSet = func(service, account, key string) error {
		saved = key
		return nil
	}
	if err := saveAPIKey("  SECRET  "); err != nil {
		t.Fatalf("saveAPIKey() error: %v", err)
	}
	if validated != "SECRET" || saved != "SECRET" {
		t.Fatalf("validated = %q, saved = %q", validated, saved)
	}
}

func TestSaveAPIKeyDoesNotPersistInvalidKey(t *testing.T) {
	withCredentialStubs(t)
	validateKey = func(string) error { return errors.New("invalid") }
	wrote := false
	keyringSet = func(service, account, key string) error {
		wrote = true
		return nil
	}
	if err := saveAPIKey("SECRET"); err == nil || wrote {
		t.Fatalf("saveAPIKey() error = %v, wrote = %t", err, wrote)
	}
}

func TestCredentialErrorsNeverContainKey(t *testing.T) {
	withCredentialStubs(t)
	validateKey = func(string) error { return errors.New("request rejected") }
	err := saveAPIKey("SUPER_SECRET")
	if err == nil || strings.Contains(err.Error(), "SUPER_SECRET") {
		t.Fatalf("saveAPIKey() error = %v", err)
	}
}

type credentialRoundTripper func(*http.Request) (*http.Response, error)

func (f credentialRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestValidateAPIKeySanitizesTransportErrors(t *testing.T) {
	secret := "SUPER_SECRET"
	originalClient := httpClient
	httpClient = &http.Client{Transport: credentialRoundTripper(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New(request.URL.String())
	})}
	t.Cleanup(func() { httpClient = originalClient })

	err := validateAPIKey(secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("validateAPIKey() error = %v", err)
	}
}

func TestValidateAPIKeyDoesNotExposeResponseBody(t *testing.T) {
	secret := "SUPER_SECRET"
	originalClient := httpClient
	httpClient = &http.Client{Transport: credentialRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("rejected " + secret)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalClient })

	err := validateAPIKey(secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("validateAPIKey() error = %v", err)
	}
}

func TestRemoveSavedAPIKeyIgnoresMissingCredential(t *testing.T) {
	withCredentialStubs(t)
	keyringDelete = func(service, account string) error { return keyring.ErrNotFound }
	if err := removeSavedAPIKey(); err != nil {
		t.Fatalf("removeSavedAPIKey() error: %v", err)
	}
}

func TestLookupSavedAPIKeyTimesOut(t *testing.T) {
	withCredentialStubs(t)
	blocked := make(chan struct{})
	keyringGet = func(service, account string) (string, error) {
		<-blocked
		return "", nil
	}
	defer close(blocked)

	started := time.Now()
	_, err := lookupSavedAPIKey(10 * time.Millisecond)
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("lookupSavedAPIKey() error = %v, duration = %s", err, time.Since(started))
	}
}
