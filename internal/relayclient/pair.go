package relayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PairingStart is what the relay gives an agent that asks to be paired.
type PairingStart struct {
	DeviceID   string    `json:"device_id"`
	Code       string    `json:"code"`
	ClaimToken string    `json:"claim_token"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// StartPairing asks the relay for a one-time code to show the user.
//
// This happens before the PC has any credentials at all, which is the point: the
// user proves they own the PC by reading a code off its screen and typing it
// into the web app, where they are already signed in.
func StartPairing(ctx context.Context, relayURL, deviceName string) (PairingStart, error) {
	body, _ := json.Marshal(map[string]string{"name": deviceName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(relayURL, "/")+"/pair/start", bytes.NewReader(body))
	if err != nil {
		return PairingStart{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return PairingStart{}, fmt.Errorf("couldn't reach the QueueUp relay at %s: %w", relayURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PairingStart{}, fmt.Errorf("the relay refused to start pairing: %s", readError(resp.Body))
	}
	var out PairingStart
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PairingStart{}, err
	}
	return out, nil
}

// WaitForPairing polls until the user types the code into the web app, then
// returns this PC's permanent token.
func WaitForPairing(ctx context.Context, relayURL, claimToken string, poll time.Duration) (string, error) {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	url := fmt.Sprintf("%s/pair/result?claim_token=%s",
		strings.TrimSuffix(relayURL, "/"), claimToken)

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// The relay being briefly unreachable during pairing is not fatal.
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(poll):
				continue
			}
		}
		var out struct {
			Done        bool   `json:"done"`
			DeviceToken string `json:"device_token"`
		}
		status := resp.StatusCode
		errText := ""
		if status == http.StatusOK {
			_ = json.NewDecoder(resp.Body).Decode(&out)
		} else {
			errText = readError(resp.Body)
		}
		resp.Body.Close()

		switch {
		case status == http.StatusOK && out.Done:
			return out.DeviceToken, nil
		case status == http.StatusOK:
			// Still waiting for the user.
		default:
			return "", fmt.Errorf("%s", errText)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(poll):
		}
	}
}

func readError(r io.Reader) string {
	var e struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r, 4096))
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(raw))
}
