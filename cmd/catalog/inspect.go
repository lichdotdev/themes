package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// report is what inspecting one repository found: the entry it would list,
// what holds it and what only warns.
type report struct {
	repo     string
	entry    pack
	problems []string
	warnings []string
}

func (r *report) problem(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func (r *report) warn(format string, args ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

// inspect reads full's newest commit on its default branch, or the commit the
// lock names for the version it carries, and checks the pack there.
func (b *builder) inspect(ctx context.Context, full string, lock versionLock) (*report, error) {
	r := &report{repo: full}
	info, err := b.repo(ctx, full)
	if err != nil {
		return nil, err
	}
	head, err := b.commit(ctx, full, info.DefaultBranch)
	if err != nil {
		return nil, err
	}
	m, err := b.manifestAt(ctx, full, head.SHA)
	if err != nil {
		r.problem("%v", err)
		return r, nil
	}
	sha, date := head.SHA, head.Commit.Committer.Date
	if locked := lock.commit(full, m.Version); locked != "" && locked != head.SHA {
		r.warn("version %s changed after it was first listed at %s; bump the version to list the change", m.Version, short(locked))
		at, err := b.commit(ctx, full, locked)
		if err != nil {
			return nil, err
		}
		sha, date = locked, at.Commit.Committer.Date
		if m, err = b.manifestAt(ctx, full, sha); err != nil {
			r.problem("%v", err)
			return r, nil
		}
	}
	r.problems = append(r.problems, checkManifest(m)...)
	themes, err := b.themesAt(ctx, full, sha)
	if err != nil {
		r.problem("%v", err)
		return r, nil
	}
	r.problems = append(r.problems, checkThemes(themes)...)
	r.entry = entryOf(info, m, sha, date, themes)
	return r, nil
}

func (b *builder) manifestAt(ctx context.Context, full, sha string) (manifest, error) {
	var m manifest
	data, err := b.file(ctx, full, sha, "lich-theme.json")
	if err != nil {
		return m, fmt.Errorf("no lich-theme.json at %s: %w", short(sha), err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("lich-theme.json: %w", err)
	}
	return m, nil
}

// themesAt reads every *.json at sha's root but the manifest, as lich does.
func (b *builder) themesAt(ctx context.Context, full, sha string) ([]theme, error) {
	files, err := b.rootFiles(ctx, full, sha)
	if err != nil {
		return nil, err
	}
	var themes []theme
	for _, f := range files {
		if f.Type != "file" || !strings.HasSuffix(f.Name, ".json") || f.Name == "lich-theme.json" {
			continue
		}
		data, err := b.file(ctx, full, sha, f.Name)
		if err != nil {
			return nil, err
		}
		var t theme
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		themes = append(themes, t)
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].ID < themes[j].ID })
	return themes, nil
}

func entryOf(info repoInfo, m manifest, sha, date string, themes []theme) pack {
	owner, name, _ := strings.Cut(info.FullName, "/")
	e := pack{
		ID: strings.TrimSuffix(strings.ToLower(name), "-lich"), Name: strings.TrimSpace(m.Name),
		Description: strings.TrimSpace(m.Description), Publisher: owner, Official: owner == "lichdotdev",
		Stars: info.Stars, Repo: "https://github.com/" + info.FullName, RepoID: info.ID,
		Version: strings.TrimPrefix(strings.TrimSpace(m.Version), "v"), Commit: sha,
		MinLichVersion: strings.TrimPrefix(m.MinLichVersion, "v"), Updated: date, Themes: themes,
	}
	if e.Description == "" {
		e.Description = info.Description
	}
	if info.License != nil && info.License.SPDX != "NOASSERTION" {
		e.License = info.License.SPDX
	}
	return e
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
