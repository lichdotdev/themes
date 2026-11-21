package main

import "encoding/json"

// catalogFormat is the newest catalog.json format this builder writes. lich
// refuses a format newer than the one it reads (docs/themes.md, Listing in the
// catalog), so bump it only for a change an older lich would misread.
const catalogFormat = 1

// catalog is catalog.json: every listed pack at the commit its version was
// first seen at, with its themes in full, so lich draws a theme before it is
// installed. lich reads it as api.ThemeCatalog; the two shapes are one.
type catalog struct {
	FormatVersion int          `json:"formatVersion"`
	Generated     string       `json:"generated"`
	Packs         []pack       `json:"packs"`
	Revoked       []revocation `json:"revoked,omitempty"`
}

// pack is one listed repository. Repo is https://github.com/<owner>/<repo>
// and Publisher its owner. Commit is what lich installs, and only that: the
// first commit the catalog saw carry Version. Held, when set, says why the
// repository's newest state is not the one listed.
type pack struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description,omitempty"`
	Publisher      string  `json:"publisher"`
	Official       bool    `json:"official,omitempty"`
	Stars          int     `json:"stars"`
	Repo           string  `json:"repo"`
	RepoID         int64   `json:"repoId"`
	License        string  `json:"license,omitempty"`
	Version        string  `json:"version"`
	Commit         string  `json:"commit"`
	MinLichVersion string  `json:"minLichVersion,omitempty"`
	Updated        string  `json:"updated"`
	Held           string  `json:"held,omitempty"`
	Themes         []theme `json:"themes"`
}

// theme is a theme file as the pack carries it, the fields lich reads.
type theme struct {
	FormatVersion int               `json:"formatVersion,omitempty"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Scheme        string            `json:"scheme"`
	Pair          string            `json:"pair,omitempty"`
	App           map[string]string `json:"app"`
	Terminal      map[string]string `json:"terminal"`
	Syntax        json.RawMessage   `json:"syntax,omitempty"`
}

// revocation is a repository taken down: lich no longer offers it.
type revocation struct {
	Repo   string `json:"repo"`
	RepoID int64  `json:"repoId,omitempty"`
	Reason string `json:"reason"`
}

// manifest is lich-theme.json, the file that makes a repository a pack.
type manifest struct {
	FormatVersion  int    `json:"formatVersion,omitempty"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	MinLichVersion string `json:"minLichVersion,omitempty"`
	Description    string `json:"description,omitempty"`
}
