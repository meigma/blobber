package sigstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// GetAmbientToken attempts to retrieve an OIDC token from the ambient CI environment.
// Currently supports GitHub Actions.
func GetAmbientToken(ctx context.Context) (string, error) {
	// Try GitHub Actions first
	token, err := getGitHubActionsToken(ctx)
	if err == nil {
		return token, nil
	}

	return "", errors.New("no ambient OIDC credentials found (not running in GitHub Actions?)")
}

// getGitHubActionsToken fetches an OIDC token from GitHub Actions.
// Requires ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN env vars.
func getGitHubActionsToken(ctx context.Context) (string, error) {
	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	if requestURL == "" || requestToken == "" {
		return "", errors.New("GitHub Actions OIDC environment variables not set")
	}

	// Add audience parameter for Sigstore
	url := requestURL + "&audience=sigstore"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub Actions OIDC request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.Value == "" {
		return "", errors.New("empty token returned from GitHub Actions")
	}

	return tokenResp.Value, nil
}
