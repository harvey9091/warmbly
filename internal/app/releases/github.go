package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Release is one GitHub release as the update surfaces read it.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

// FetchReleases lists the newest releases of owner/repo. token is optional and
// only raises the unauthenticated rate limit.
func FetchReleases(ctx context.Context, client *http.Client, repo, token string) ([]Release, error) {
	if repo == "" {
		return nil, errors.New("RELEASES_GITHUB_REPO not set")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	url := "https://api.github.com/repos/" + repo + "/releases?per_page=30"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return releases, nil
}

// PickChannelHeads returns the most recent published release for each channel:
//   - stable: latest non-prerelease, non-draft
//   - dev:    latest published (including prereleases)
func PickChannelHeads(releases []Release) (stable, dev *Release) {
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].PublishedAt.After(releases[j].PublishedAt)
	})
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if dev == nil {
			dev = r
		}
		if !r.Prerelease && stable == nil {
			stable = r
		}
		if dev != nil && stable != nil {
			break
		}
	}
	return
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
