// Command catalog builds catalog.json from the repositories themes.json lists,
// and checks one repository the way the build would.
//
//	catalog build
//	catalog check <owner>/<repo>
//
// GITHUB_TOKEN, when set, authenticates the GitHub calls.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"
)

const usage = "usage: catalog build | catalog check <owner>/<repo>"

type builder struct {
	api, raw string
	token    string
	http     *http.Client
}

func main() {
	b := &builder{
		api: "https://api.github.com", raw: "https://raw.githubusercontent.com",
		token: os.Getenv("GITHUB_TOKEN"), http: &http.Client{Timeout: time.Minute},
	}
	os.Exit(run(context.Background(), b, ".", os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, b *builder, dir string, args []string, stdout, stderr io.Writer) int {
	var err error
	ok := true
	switch {
	case len(args) == 1 && args[0] == "build":
		err = b.build(ctx, dir, stdout)
	case len(args) == 2 && args[0] == "check":
		ok, err = b.check(ctx, dir, args[1], stdout)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !ok {
		return 1
	}
	return 0
}

func (b *builder) build(ctx context.Context, dir string, log io.Writer) error {
	s, err := loadState(dir)
	if err != nil {
		return err
	}
	ids := newClaims()
	packs := []pack{}
	for _, full := range s.repos {
		r, err := b.inspect(ctx, full, s.lock)
		if err != nil {
			return fmt.Errorf("%s: %w", full, err)
		}
		if rv := s.revokedFor("https://github.com/"+full, r.entry.RepoID); rv != nil {
			fmt.Fprintf(log, "%s: revoked: %s\n", full, rv.Reason)
			continue
		}
		ids.settle(r)
		printReport(log, r)
		e, ok := s.accept(r)
		if !ok {
			continue
		}
		ids.take(e)
		packs = append(packs, e)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].ID < packs[j].ID })
	cat := catalog{
		FormatVersion: catalogFormat, Generated: time.Now().UTC().Format(time.RFC3339),
		Packs: packs, Revoked: s.revoked,
	}
	if sameListing(cat, s.previous) {
		cat.Generated = s.previous.Generated
	}
	return s.save(cat)
}

// check runs the build's checks on one repository against the catalog as it
// stands, and prints every problem. It is false when any holds the listing.
func (b *builder) check(ctx context.Context, dir, full string, out io.Writer) (bool, error) {
	s, err := loadState(dir)
	if err != nil {
		return false, err
	}
	r, err := b.inspect(ctx, full, s.lock)
	if err != nil {
		return false, err
	}
	if rv := s.revokedFor("https://github.com/"+full, r.entry.RepoID); rv != nil {
		r.problem("revoked: %s", rv.Reason)
	}
	ids := newClaims()
	for _, p := range s.previous.Packs {
		ids.take(p)
	}
	ids.settle(r)
	printReport(out, r)
	return len(r.problems) == 0, nil
}

func printReport(w io.Writer, r *report) {
	for _, p := range r.problems {
		fmt.Fprintf(w, "%s: problem: %s\n", r.repo, p)
	}
	for _, m := range r.warnings {
		fmt.Fprintf(w, "%s: warning: %s\n", r.repo, m)
	}
	if len(r.problems) == 0 {
		fmt.Fprintf(w, "%s: ok: %s %s, %d themes at %s\n", r.repo, r.entry.ID, r.entry.Version, len(r.entry.Themes), short(r.entry.Commit))
	}
}
