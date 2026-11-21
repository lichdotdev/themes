package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxFile bounds every file the build reads from a pack, lich's own limit for
// a theme file.
const maxFile = 1 << 20

// repoInfo is what the build reads of a repository from GitHub's API.
type repoInfo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Stars         int    `json:"stargazers_count"`
	License       *struct {
		SPDX string `json:"spdx_id"`
	} `json:"license"`
}

// commitInfo is a commit as GitHub's API names it.
type commitInfo struct {
	SHA    string `json:"sha"`
	Commit struct {
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

// contentEntry is one file at the root of a commit.
type contentEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (b *builder) repo(ctx context.Context, full string) (repoInfo, error) {
	var r repoInfo
	return r, b.getJSON(ctx, b.api+"/repos/"+full, &r)
}

func (b *builder) commit(ctx context.Context, full, ref string) (commitInfo, error) {
	var c commitInfo
	return c, b.getJSON(ctx, b.api+"/repos/"+full+"/commits/"+ref, &c)
}

func (b *builder) rootFiles(ctx context.Context, full, sha string) ([]contentEntry, error) {
	var entries []contentEntry
	return entries, b.getJSON(ctx, b.api+"/repos/"+full+"/contents?ref="+sha, &entries)
}

// file reads path at sha, held to maxFile.
func (b *builder) file(ctx context.Context, full, sha, path string) ([]byte, error) {
	return b.get(ctx, fmt.Sprintf("%s/%s/%s/%s", b.raw, full, sha, path), false)
}

func (b *builder) getJSON(ctx context.Context, url string, v any) error {
	data, err := b.get(ctx, url, true)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

func (b *builder) get(ctx context.Context, url string, api bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lichdotdev-themes-catalog")
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	if b.token != "" && (api || strings.HasPrefix(url, b.raw)) {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("%s is larger than %d KiB", url, maxFile>>10)
	}
	return data, nil
}
