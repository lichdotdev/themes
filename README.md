# lich theme catalog

The list of themes that [lich](https://github.com/lichdotdev/lich)'s Settings › Appearance › Themes shows. The
build reads every repository in `themes.json` and writes `catalog.json`, served from
`https://raw.githubusercontent.com/lichdotdev/themes/main/catalog.json`. The catalog carries every theme in full, so
lich draws a theme's preview before it is installed, and installs a pack at the commit listed here, nothing else.
The format is described in [Listing in the catalog](https://github.com/lichdotdev/lich/blob/main/docs/themes.md#listing-in-the-catalog).

## Getting listed

1. Publish your theme the way lich installs one from git: a public repository with `lich-theme.json` (`name`,
   `version` as `MAJOR.MINOR.PATCH`, optional `minLichVersion` and `description`) and one `<id>.json` per variant
   next to it. [rose-pine-lich](https://github.com/lichdotdev/rose-pine-lich) is the shape to copy.
2. Open a pull request adding `"<owner>/<repo>"` to `themes.json`. Its check runs on your default branch and
   comments with what fails; push to the pull request to run it again. A maintainer approves the first listing,
   and every later version is picked up within an hour.

You can run the same check yourself: `go run ./cmd/catalog check <owner>/<repo>`.

## What is checked

- `lich-theme.json` has a name and a `MAJOR.MINOR.PATCH` version, and a description of at most 200 characters.
- The pack has 1 to 32 themes. Each has an id of lowercase letters, digits, dots, dashes or underscores that is
  not `light`, `dark`, `system` or `match`, a name, a scheme of `dark` or `light`, a `pair` that is in the same
  pack, the app colors the previews draw with and all 16 terminal colors.
- No other listed pack has claimed the pack's id (the repository name without `-lich`) or any of its theme ids.

lich validates every color again when it installs, and refuses a pack written for a newer lich by its
`minLichVersion`.

## What the catalog holds to

- `versions.lock.json` records the commit each version was first listed at, and that record never changes. A
  version whose files change afterwards stays listed at its first commit, with a warning in the build log: bump the
  version to list the change.
- A new version that fails a check keeps the pack at its last good entry, with the reason in `held`. A repository
  never listed is left out until it passes.
- The build fails, and commits nothing, when GitHub itself does not answer; the next run retries.

## Taking a pack down

A maintainer who confirms deception or infringement adds the repository to `revoked.json` with `repo`, `repoId` and
a `reason`; the next build drops it, and lich stops offering it.
