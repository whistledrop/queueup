package relayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SendReport uploads a problem report to the relay, filed under the account
// this PC is linked to.
//
// It is an ordinary HTTPS request rather than a message on the agent's
// socket: a report is most needed exactly when something is wrong, and that
// includes the socket being down.
func SendReport(ctx context.Context, relayURL, deviceToken, agentVersion string, report []byte) error {
	if deviceToken == "" {
		return errors.New("this PC isn't linked to an account yet")
	}
	endpoint, err := reportURL(relayURL)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{
		"report":        string(report),
		"agent_version": agentVersion,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return errors.New("couldn't reach QueueUp")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Error != "" {
		return errors.New(strings.TrimSuffix(body.Error, "."))
	}
	return fmt.Errorf("QueueUp said %d", resp.StatusCode)
}

// reportURL turns the saved relay address, which may be written as a socket
// address, into the report endpoint.
func reportURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%q is not a relay address", base)
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	case "https", "http":
	default:
		return "", fmt.Errorf("%q is not a relay address", base)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/agent/report"
	u.RawQuery = ""
	return u.String(), nil
}
