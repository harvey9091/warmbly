package emailverify

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

// CleanMyList verifies addresses using the workspace's API key.
type CleanMyList struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewCleanMyList(apiKey, baseURL string) *CleanMyList {
	if baseURL == "" {
		baseURL = "https://www.cleanmylist.io"
	}
	return &CleanMyList{apiKey: strings.TrimSpace(apiKey), baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 60 * time.Second}}
}

func (c *CleanMyList) Check(ctx context.Context, email string) (Result, error) {
	res := Result{Email: strings.ToLower(strings.TrimSpace(email)), CheckedAt: time.Now().UTC(), Status: StatusUnknown, Provider: ProviderCleanMyList}
	body, _ := json.Marshal(map[string]any{"email": res.Email, "smtp_probe": true})
	var out struct {
		Verdict    string `json:"verdict"`
		Score      int    `json:"score"`
		Reason     string `json:"reason"`
		ReasonCode string `json:"reason_code"`
		Checks     []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			ReasonCode string `json:"reason_code"`
		} `json:"checks"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/verify", body, &out); err != nil {
		res.Reason = err.Error()
		return res, err
	}
	v, ok := NormalizeExternal(ProviderCleanMyList, out.Verdict)
	if !ok {
		return res, fmt.Errorf("cleanmylist: unrecognised verdict %q", out.Verdict)
	}
	res.Status = v.Status
	switch out.ReasonCode {
	case "catch_all":
		res.SubStatus = SubStatusCatchAll
	case "role_account":
		res.SubStatus = SubStatusRole
	case "disposable":
		res.SubStatus = SubStatusDisposable
	case "invalid_syntax":
		res.SubStatus = SubStatusSyntax
	case "no_mx", "null_mx":
		res.SubStatus = SubStatusNoMX
	}
	res.IsCatchAll = res.SubStatus == SubStatusCatchAll
	res.Confidence = out.Score
	res.Reason = "cleanmylist: " + out.Reason
	if out.ReasonCode != "" {
		res.Reason += " (" + out.ReasonCode + ")"
	}
	for _, check := range out.Checks {
		if check.Name == "catch_all" && check.ReasonCode == "catch_all" {
			res.IsCatchAll = true
		}
		if check.Name == "mx_records" && check.Status == "pass" {
			res.HasMX = true
		}
	}
	return res, nil
}

// Account validates the key without spending allowance; no balance API exists.
func (c *CleanMyList) Account(ctx context.Context) (*int, error) {
	var out struct {
		Jobs json.RawMessage `json:"jobs"`
	}
	err := c.request(ctx, http.MethodGet, "/v1/jobs", nil, &out)
	if err == nil && (len(out.Jobs) == 0 || out.Jobs[0] != '[') {
		err = fmt.Errorf("cleanmylist: unreadable account response")
	}
	return nil, err
}

func (c *CleanMyList) request(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cleanmylist: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrProviderKey
	case http.StatusForbidden:
		return fmt.Errorf("verify your CleanMyList account email before connecting: %w", ErrProviderKey)
	case http.StatusPaymentRequired:
		return ErrProviderCredits
	case http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return fmt.Errorf("cleanmylist: unreadable response: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("cleanmylist: HTTP %d", resp.StatusCode)
	}
}
