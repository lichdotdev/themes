package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHub serves repositories the way the API and raw.githubusercontent do:
// each repository has commits, newest last, each with its root files.
type fakeGitHub struct {
	repos map[string]*fakeRepo
}

type fakeRepo struct {
	id      int64
	stars   int
	commits []fakeCommit
}

type fakeCommit struct {
	sha   string
	files map[string]string
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		full := r.PathValue("owner") + "/" + r.PathValue("repo")
		repo := f.repos[full]
		if repo == nil {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"id": repo.id, "full_name": full, "default_branch": "main", "stargazers_count": repo.stars,
			"license": map[string]string{"spdx_id": "MIT"}})
	})
	mux.HandleFunc("/api/repos/{owner}/{repo}/commits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		c := f.find(r.PathValue("owner")+"/"+r.PathValue("repo"), r.PathValue("ref"))
		if c == nil {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"sha": c.sha, "commit": map[string]any{"committer": map[string]string{"date": "2026-11-0" + c.sha[len(c.sha)-1:] + "T00:00:00Z"}}})
	})
	mux.HandleFunc("/api/repos/{owner}/{repo}/contents", func(w http.ResponseWriter, r *http.Request) {
		c := f.find(r.PathValue("owner")+"/"+r.PathValue("repo"), r.URL.Query().Get("ref"))
		var entries []map[string]string
		for name := range c.files {
			entries = append(entries, map[string]string{"name": name, "type": "file"})
		}
		writeTestJSON(w, entries)
	})
	mux.HandleFunc("/raw/{owner}/{repo}/{sha}/{path}", func(w http.ResponseWriter, r *http.Request) {
		c := f.find(r.PathValue("owner")+"/"+r.PathValue("repo"), r.PathValue("sha"))
		body, ok := c.files[r.PathValue("path")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	})
	return mux
}

// find is the commit ref names: main is the newest, else the sha itself.
func (f *fakeGitHub) find(full, ref string) *fakeCommit {
	repo := f.repos[full]
	if repo == nil {
		return nil
	}
	if ref == "main" {
		return &repo.commits[len(repo.commits)-1]
	}
	for i := range repo.commits {
		if repo.commits[i].sha == ref {
			return &repo.commits[i]
		}
	}
	return nil
}

func writeTestJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func themeFile(id, scheme string) string {
	app := map[string]string{}
	for _, k := range appKeys {
		app[k] = "#101012"
	}
	term := map[string]string{}
	for _, k := range terminalKeys {
		term[k] = "#ffffff"
	}
	data, _ := json.Marshal(map[string]any{"id": id, "name": strings.ToUpper(id[:1]) + id[1:], "scheme": scheme, "app": app, "terminal": term})
	return string(data)
}

func packFiles(version string, themes ...string) map[string]string {
	files := map[string]string{"lich-theme.json": `{"name":"Pack","version":"` + version + `","description":"A pack."}`}
	for _, id := range themes {
		files[id+".json"] = themeFile(id, "dark")
	}
	return files
}

func testRig(t *testing.T, f *fakeGitHub, repos ...string) (*builder, string) {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	list, _ := json.Marshal(repos)
	if err := os.WriteFile(filepath.Join(dir, listFile), list, 0o644); err != nil {
		t.Fatal(err)
	}
	return &builder{api: srv.URL + "/api", raw: srv.URL + "/raw", http: srv.Client()}, dir
}

func readCatalog(t *testing.T, dir string) catalog {
	t.Helper()
	var c catalog
	if err := readJSON(dir, catalogFile, &c, false); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheBuildListsEachPackWithItsThemesAtItsNewestCommit(t *testing.T) {
	f := &fakeGitHub{repos: map[string]*fakeRepo{
		"lichdotdev/rose-lich": {id: 1, stars: 3, commits: []fakeCommit{{sha: "a1", files: packFiles("1.0.0", "rose", "rose-moon")}}},
		"someone/dusk-lich":    {id: 2, commits: []fakeCommit{{sha: "b1", files: packFiles("v2.1.0", "dusk")}}},
	}}
	b, dir := testRig(t, f, "lichdotdev/rose-lich", "someone/dusk-lich")
	var log bytes.Buffer
	if err := b.build(context.Background(), dir, &log); err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, dir)
	if c.FormatVersion != catalogFormat || len(c.Packs) != 2 {
		t.Fatalf("catalog %+v\n%s", c, log.String())
	}
	dusk, rose := c.Packs[0], c.Packs[1]
	if rose.ID != "rose" || !rose.Official || rose.Stars != 3 || rose.Commit != "a1" || len(rose.Themes) != 2 || rose.License != "MIT" {
		t.Errorf("rose = %+v", rose)
	}
	if dusk.ID != "dusk" || dusk.Official || dusk.Version != "2.1.0" || dusk.Publisher != "someone" {
		t.Errorf("dusk = %+v: the version loses its v, someone is no official publisher", dusk)
	}
}

func TestAVersionStaysAtTheCommitItWasFirstListedAt(t *testing.T) {
	repo := &fakeRepo{id: 1, commits: []fakeCommit{{sha: "a1", files: packFiles("1.0.0", "rose")}}}
	f := &fakeGitHub{repos: map[string]*fakeRepo{"o/rose-lich": repo}}
	b, dir := testRig(t, f, "o/rose-lich")
	ctx := context.Background()
	if err := b.build(ctx, dir, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	repo.commits = append(repo.commits, fakeCommit{sha: "a2", files: packFiles("1.0.0", "rose", "rose-dawn")})
	var log bytes.Buffer
	if err := b.build(ctx, dir, &log); err != nil {
		t.Fatal(err)
	}
	if p := readCatalog(t, dir).Packs[0]; p.Commit != "a1" || len(p.Themes) != 1 || !strings.Contains(log.String(), "bump the version") {
		t.Errorf("pack at %s with %d themes, log %q: a changed 1.0.0 is still the first one", p.Commit, len(p.Themes), log.String())
	}
	repo.commits = append(repo.commits, fakeCommit{sha: "a3", files: packFiles("1.1.0", "rose", "rose-dawn")})
	if err := b.build(ctx, dir, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if p := readCatalog(t, dir).Packs[0]; p.Commit != "a3" || p.Version != "1.1.0" {
		t.Errorf("pack at %s %s, want the new version listed", p.Commit, p.Version)
	}
}

func TestABrokenReleaseKeepsThePreviousListingHeld(t *testing.T) {
	repo := &fakeRepo{id: 1, commits: []fakeCommit{{sha: "a1", files: packFiles("1.0.0", "rose")}}}
	f := &fakeGitHub{repos: map[string]*fakeRepo{"o/rose-lich": repo}}
	b, dir := testRig(t, f, "o/rose-lich")
	ctx := context.Background()
	if err := b.build(ctx, dir, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	broken := packFiles("1.1.0")
	broken["dark.json"] = themeFile("dark", "dark")
	repo.commits = append(repo.commits, fakeCommit{sha: "a2", files: broken})
	if err := b.build(ctx, dir, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if p := readCatalog(t, dir).Packs[0]; p.Commit != "a1" || !strings.Contains(p.Held, "lich's own") {
		t.Errorf("pack %+v: want 1.0.0 still listed, held for the reserved id", p)
	}
}

func TestTheFirstPackToClaimAThemeIDKeepsIt(t *testing.T) {
	f := &fakeGitHub{repos: map[string]*fakeRepo{
		"a/one-lich": {id: 1, commits: []fakeCommit{{sha: "a1", files: packFiles("1.0.0", "night")}}},
		"b/two-lich": {id: 2, commits: []fakeCommit{{sha: "b1", files: packFiles("1.0.0", "night")}}},
	}}
	b, dir := testRig(t, f, "a/one-lich", "b/two-lich")
	var log bytes.Buffer
	if err := b.build(context.Background(), dir, &log); err != nil {
		t.Fatal(err)
	}
	if c := readCatalog(t, dir); len(c.Packs) != 1 || c.Packs[0].ID != "one" || !strings.Contains(log.String(), `theme id "night" is taken`) {
		t.Errorf("packs %+v, log %q", c.Packs, log.String())
	}
}

func TestARevokedRepositoryIsNotListed(t *testing.T) {
	f := &fakeGitHub{repos: map[string]*fakeRepo{"o/rose-lich": {id: 7, commits: []fakeCommit{{sha: "a1", files: packFiles("1.0.0", "rose")}}}}}
	b, dir := testRig(t, f, "o/rose-lich")
	if err := writeJSON(dir, revokedFile, []revocation{{RepoID: 7, Reason: "stolen"}}); err != nil {
		t.Fatal(err)
	}
	if err := b.build(context.Background(), dir, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	if c := readCatalog(t, dir); len(c.Packs) != 0 || len(c.Revoked) != 1 {
		t.Errorf("catalog %+v: a revoked repository goes, its revocation stays", c)
	}
}

func TestCheckReportsEveryProblemOfOneRepository(t *testing.T) {
	files := packFiles("1.0", "Bad ID")
	files["light.json"] = themeFile("light", "dusky")
	f := &fakeGitHub{repos: map[string]*fakeRepo{"o/bad-lich": {id: 1, commits: []fakeCommit{{sha: "a1", files: files}}}}}
	b, dir := testRig(t, f, "o/bad-lich")
	var out, errs bytes.Buffer
	if code := run(context.Background(), b, dir, []string{"check", "o/bad-lich"}, &out, &errs); code != 1 {
		t.Fatalf("exit %d, want 1:\n%s%s", code, out.String(), errs.String())
	}
	for _, want := range []string{"MAJOR.MINOR.PATCH", `id "Bad ID"`, `id "light" is lich's own`, `scheme "dusky"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report misses %q:\n%s", want, out.String())
		}
	}
	if code := run(context.Background(), b, dir, []string{"nope"}, &out, &errs); code != 2 {
		t.Errorf("a bad command exits %d, want 2", code)
	}
}

func TestAThemeMustNameItsPairAndEveryTerminalColor(t *testing.T) {
	th := theme{ID: "x", Name: "X", Scheme: "dark", Pair: "y", App: map[string]string{}, Terminal: map[string]string{"background": "#000"}}
	got := strings.Join(checkThemes([]theme{th}), "\n")
	for _, want := range []string{`pair "y"`, "app.background is missing", "terminal.brightWhite is missing"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems miss %q:\n%s", want, got)
		}
	}
	if len(checkThemes(nil)) != 1 {
		t.Error("a pack with no theme passes")
	}
}
