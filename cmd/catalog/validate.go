package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The limits lich holds a pack to (internal/themes in lichdotdev/lich). This
// check is structural: lich validates every color when it installs, and a pack
// it refuses there is one this builder should have held, so keep the two close.
const (
	maxThemes      = 32
	maxIDLength    = 64
	maxNameLength  = 128
	maxDescription = 200
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	semverPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)
	// reservedIDs are the bundled themes' ids and the window's own names.
	reservedIDs = map[string]bool{"light": true, "dark": true, "system": true, "match": true}
	// terminalKeys are the colors a theme's terminal palette must name.
	terminalKeys = []string{
		"background", "foreground",
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite",
	}
	// appKeys are the app colors the catalog's previews draw with; lich asks
	// for the rest when it installs.
	appKeys = []string{"background", "foreground", "card", "sidebar", "accent", "muted-foreground", "primary", "border"}
)

// checkManifest names what is wrong with a pack's manifest, nil when nothing.
func checkManifest(m manifest) []string {
	var out []string
	if strings.TrimSpace(m.Name) == "" {
		out = append(out, "lich-theme.json has no name")
	} else if utf8.RuneCountInString(m.Name) > maxNameLength {
		out = append(out, fmt.Sprintf("lich-theme.json name is longer than %d characters", maxNameLength))
	}
	if !semverPattern.MatchString(strings.TrimSpace(m.Version)) {
		out = append(out, fmt.Sprintf("lich-theme.json version %q is not MAJOR.MINOR.PATCH", m.Version))
	}
	if m.MinLichVersion != "" && !semverPattern.MatchString(m.MinLichVersion) {
		out = append(out, fmt.Sprintf("lich-theme.json minLichVersion %q is not MAJOR.MINOR.PATCH", m.MinLichVersion))
	}
	if utf8.RuneCountInString(m.Description) > maxDescription {
		out = append(out, fmt.Sprintf("lich-theme.json description is longer than %d characters", maxDescription))
	}
	return out
}

// checkThemes names what is wrong with a pack's themes, nil when nothing.
func checkThemes(themes []theme) []string {
	var out []string
	switch {
	case len(themes) == 0:
		return []string{"the pack has no theme files next to lich-theme.json"}
	case len(themes) > maxThemes:
		out = append(out, fmt.Sprintf("the pack has %d themes, more than %d", len(themes), maxThemes))
	}
	ids := map[string]bool{}
	for _, t := range themes {
		ids[t.ID] = true
	}
	for _, t := range themes {
		out = append(out, checkTheme(t, ids)...)
	}
	return out
}

func checkTheme(t theme, ids map[string]bool) []string {
	var out []string
	name := t.ID
	if name == "" {
		name = t.Name
	}
	bad := func(format string, args ...any) { out = append(out, name+": "+fmt.Sprintf(format, args...)) }
	switch {
	case !idPattern.MatchString(t.ID) || len(t.ID) > maxIDLength:
		bad("id %q must be lowercase letters, digits, dots, dashes or underscores, at most %d long", t.ID, maxIDLength)
	case reservedIDs[t.ID]:
		bad("id %q is lich's own", t.ID)
	}
	if strings.TrimSpace(t.Name) == "" || utf8.RuneCountInString(t.Name) > maxNameLength {
		bad("name must be 1 to %d characters", maxNameLength)
	}
	if t.Scheme != "dark" && t.Scheme != "light" {
		bad("scheme %q must be dark or light", t.Scheme)
	}
	if t.Pair != "" && !ids[t.Pair] {
		bad("pair %q is not a theme in this pack", t.Pair)
	}
	for _, k := range appKeys {
		if t.App[k] == "" {
			bad("app.%s is missing", k)
		}
	}
	for _, k := range terminalKeys {
		if t.Terminal[k] == "" {
			bad("terminal.%s is missing", k)
		}
	}
	return out
}
