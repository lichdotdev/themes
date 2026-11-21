package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// The repository's files: the list a pull request edits, what the build
// writes, and what only a maintainer writes.
const (
	listFile    = "themes.json"
	catalogFile = "catalog.json"
	lockFile    = "versions.lock.json"
	revokedFile = "revoked.json"
)

// versionLock is versions.lock.json: for each repository, the commit each
// version was first listed at. The first one wins, so a version never names
// two different contents.
type versionLock map[string]map[string]string

func (l versionLock) commit(full, version string) string {
	return l[strings.ToLower(full)][strings.TrimPrefix(version, "v")]
}

func (l versionLock) record(full, version, sha string) {
	key := strings.ToLower(full)
	if l[key] == nil {
		l[key] = map[string]string{}
	}
	if _, ok := l[key][version]; !ok {
		l[key][version] = sha
	}
}

// state is the repository as the build reads it.
type state struct {
	dir      string
	repos    []string
	revoked  []revocation
	lock     versionLock
	previous catalog
}

func loadState(dir string) (*state, error) {
	s := &state{dir: dir, lock: versionLock{}}
	if err := readJSON(dir, listFile, &s.repos, false); err != nil {
		return nil, err
	}
	for _, full := range s.repos {
		if strings.Count(full, "/") != 1 || strings.ContainsAny(full, " \t") {
			return nil, fmt.Errorf("%s: %q is not owner/repo", listFile, full)
		}
	}
	for _, f := range []struct {
		name string
		v    any
	}{{revokedFile, &s.revoked}, {lockFile, &s.lock}, {catalogFile, &s.previous}} {
		if err := readJSON(dir, f.name, f.v, true); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func readJSON(dir, name string, v any, optional bool) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if optional && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func writeJSON(dir, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o644)
}

// revokedFor is the revocation that names repoURL or its repository id.
func (s *state) revokedFor(repoURL string, id int64) *revocation {
	for i, r := range s.revoked {
		if strings.EqualFold(r.Repo, repoURL) || (id != 0 && r.RepoID == id) {
			return &s.revoked[i]
		}
	}
	return nil
}

// previousEntry is the entry the last catalog listed repoURL under.
func (s *state) previousEntry(repoURL string) (pack, bool) {
	for _, p := range s.previous.Packs {
		if strings.EqualFold(p.Repo, repoURL) {
			return p, true
		}
	}
	return pack{}, false
}

// claims are the pack and theme ids the packs listed so far took.
type claims struct{ packs, themes map[string]string }

func newClaims() claims { return claims{packs: map[string]string{}, themes: map[string]string{}} }

// settle adds the problems r's ids make against what was listed before it:
// the first repository to claim an id keeps it.
func (c claims) settle(r *report) {
	e := r.entry
	if other, ok := c.packs[e.ID]; ok && !strings.EqualFold(other, e.Repo) {
		r.problem("pack id %q is taken by %s", e.ID, other)
	}
	for _, t := range e.Themes {
		if other, ok := c.themes[t.ID]; ok && !strings.EqualFold(other, e.Repo) {
			r.problem("theme id %q is taken by %s", t.ID, other)
		}
	}
}

func (c claims) take(e pack) {
	c.packs[e.ID] = e.Repo
	for _, t := range e.Themes {
		c.themes[t.ID] = e.Repo
	}
}

// accept is the entry r lists: its own when nothing holds it, the previous
// one marked held when something does and one was listed, false otherwise.
func (s *state) accept(r *report) (pack, bool) {
	if len(r.problems) == 0 {
		s.lock.record(r.repo, r.entry.Version, r.entry.Commit)
		return r.entry, true
	}
	prev, ok := s.previousEntry("https://github.com/" + r.repo)
	if !ok {
		return pack{}, false
	}
	prev.Held = r.problems[0]
	return prev, true
}

// sameListing reports whether cat lists what prev did, Generated aside, so a
// build that found nothing new leaves the file alone.
func sameListing(cat, prev catalog) bool {
	cat.Generated, prev.Generated = "", ""
	return reflect.DeepEqual(cat, prev)
}

func (s *state) save(cat catalog) error {
	if err := writeJSON(s.dir, catalogFile, cat); err != nil {
		return err
	}
	return writeJSON(s.dir, lockFile, s.lock)
}
