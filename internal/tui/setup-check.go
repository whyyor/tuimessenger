package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// backendFeature is what a tuimessenger backend announces in /versions; a plain Matrix server doesn't.
const backendFeature = "com.tuimessenger.no_e2ee"

const backendCheckTimeout = 5 * time.Second

var (
	errBackendUnreachable = errors.New("backend unreachable")
	errNotTuimessenger    = errors.New("not a tuimessenger backend")
	errLocalStart         = errors.New("couldn't start the backend here")
)

// normalizeBackendURL accepts "pi:8008" or "http://pi:8008/" and returns "http://pi:8008".
func normalizeBackendURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("enter the backend's address, like http://pi:8008")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%q isn't an address like http://pi:8008", raw)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// CheckBackend is checkBackend for the client's start-up: is a tuimessenger backend answering at base?
func CheckBackend(ctx context.Context, base string) error { return checkBackend(ctx, base) }

// IsUnreachable reports whether CheckBackend found nothing answering.
func IsUnreachable(err error) bool { return errors.Is(err, errBackendUnreachable) }

// checkBackend makes sure base answers, and answers like a tuimessenger backend rather than another server.
func checkBackend(ctx context.Context, base string) error {
	ctx, cancel := context.WithTimeout(ctx, backendCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/_matrix/client/versions", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errBackendUnreachable, err)
	}
	defer resp.Body.Close()
	var versions struct {
		UnstableFeatures map[string]bool `json:"unstable_features"`
	}
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&versions) != nil ||
		!versions.UnstableFeatures[backendFeature] {
		return errNotTuimessenger
	}
	return nil
}

// setupErrorText turns what went wrong during setup into a sentence that says what to do.
func setupErrorText(mode setupMode, base string, err error) string {
	switch {
	case errors.Is(err, errLocalStart):
		return "Couldn't start the backend on this machine: " + strings.TrimPrefix(err.Error(), errLocalStart.Error()+": ")
	case errors.Is(err, errBackendUnreachable) && mode == setupComplete:
		return fmt.Sprintf("No tuimessenger backend is running on this machine (%s). Start it with `tuimessenger`, then press Enter to try again.", base)
	case errors.Is(err, errBackendUnreachable):
		return fmt.Sprintf("Can't reach %s. Check the address, that the backend is running, and that this machine can reach it (e.g. Tailscale is up).", base)
	case errors.Is(err, errNotTuimessenger):
		return fmt.Sprintf("%s answers, but it isn't a tuimessenger backend. Check the address and port.", base)
	case strings.Contains(err.Error(), "M_FORBIDDEN"):
		return "Wrong username or password."
	default:
		return "Login failed: " + err.Error()
	}
}
