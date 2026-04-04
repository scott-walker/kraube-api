Release Kraube API — full production release pipeline.

## Input

$ARGUMENTS — optional version override (e.g. "0.2.0" or "patch"/"minor"/"major"). If empty, version is determined automatically from the diff.

## Instructions

Execute the full release pipeline for the Kraube API project. Follow every step in order. Do not skip phases.

### Phase 0: Pre-release QA

1. Run `go vet ./...` — must be clean
2. Run `go build ./...` — must compile
3. Run `go test ./...` — all tests pass
4. Run `go test -race ./...` — no race conditions
5. Test CLI manually:
   - `go run ./cmd/kraube/ --help` — prints usage
6. Grep entire repo for "kraube-go" — must return zero matches (old branding). If found, fix before proceeding.
7. Grep docs and code for standalone "Kraube" without "API" in prose (titles, descriptions) — product name is "Kraube API". Code identifiers (`kraube.NewClient`, package name) stay lowercase.

If any check fails — STOP and report.

### Phase 1: Version bump

1. Determine the new version:
   - If $ARGUMENTS is a semver string (e.g. "0.2.0"), use it directly
   - If $ARGUMENTS is "patch"/"minor"/"major", read current version and increment
   - If $ARGUMENTS is empty, auto-detect from diff:
     a. Get last tag: `git describe --tags --abbrev=0 2>/dev/null` (if no tags, current is 0.0.0)
     b. Get full diff: `git diff <last_tag>..HEAD --stat` and `git log --oneline <last_tag>..HEAD`
     c. Analyze changes and apply these rules:
        - **major** (X.0.0) if ANY of: TokenProvider interface changed, public Client struct fields removed/renamed, Go module path changed, existing Option functions removed, breaking changes to MessageRequest/MessageResponse types
        - **minor** (0.X.0) if ANY of: new TokenProvider implementations, new Option functions (With*), new API coverage (tool types, content block types), new CLI commands, significant new feature
        - **patch** (0.0.X) if ALL changes are: bug fixes, documentation updates, internal refactors, dependency updates, CI/CD changes, branding fixes, protocol updates, performance improvements
     d. Present the detected version to the user with reasoning before proceeding
2. Update CHANGELOG.md:
   - Add a section for the new version with today's date
   - List changes from `git log --oneline $(git describe --tags --abbrev=0 2>/dev/null || echo "")..HEAD`
   - Categorize: Added, Changed, Fixed, Removed
   - If breaking changes exist, add a Breaking section
3. Run `go build ./...` again to verify

### Phase 2: Branding & Documentation audit

1. Grep the entire repo for old brand names ("kraube-go"). Fix any found.
2. Verify all user-facing prose says "Kraube API" (not just "Kraube" or "kraube"). Code identifiers stay lowercase.
3. Check that README.md quick start, installation command, and examples are current
4. Check that docs/ pages reference correct APIs, types, and functions
5. Verify brand image `assets/brand.png` is referenced in README header
6. Verify badges in README point to correct repo (scott-walker/kraube-api)
7. Check that `go get github.com/scott-walker/kraube-api` matches go.mod module path

### Phase 3: Documentation site & Landing page

1. Check that `site/` builds: `cd site && npm install && npm run build`
2. Verify landing page (site/index.md + Landing.vue):
   - Brand logo present (brand-int-text-2.png from assets)
   - Install command matches current module path
   - Features list is current
   - Links work (GitHub, Get Started, API Reference)
   - Uses brandbook colors: navy #1e293b, accent #8b9dc3, bg #cbd5e1
   - Font: Alegreya Sans SC for headings (brandbook font)
   - Landing fits one screen, no scroll, no header/footer
   - Light theme only, no dark mode toggle
3. Verify guide pages are up to date:
   - getting-started.md — install command, first example
   - token-provider.md — all current providers listed
   - Any new features have corresponding guide pages
4. Verify reference pages:
   - api.md — matches current API coverage in docs/api-coverage.md
   - options.md — all With* options listed
   - cli.md — all commands and flags
   - changelog.md — matches CHANGELOG.md
5. Update site/reference/changelog.md with new version entry

### Phase 4: README & Release Notes

Generate comprehensive release notes for the GitHub Release:

1. Write a summary section (2-3 sentences) describing the release
2. List highlights (key features or fixes)
3. Include full changelog from CHANGELOG.md for this version
4. Add installation instructions:
   ```
   go get github.com/scott-walker/kraube-api@vX.Y.Z
   ```
5. Add link to documentation site: `https://scott-walker.github.io/kraube-api/`
6. If breaking changes: add Migration Guide section explaining what to change

Save release notes to a temporary file for Phase 6.

### Phase 5: Commit & Tag

1. Stage all changed files: `git add -A`
2. Review staged changes with `git diff --cached --stat`
3. Create commit: "release: vX.Y.Z" with summary of changes
4. Create annotated tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
5. Show the commit and tag for user confirmation before pushing

### Phase 6: Push & Publish

After user confirms:

1. `git push origin main`
2. `git push origin vX.Y.Z`
3. CI (GitHub Actions) handles:
   - GoReleaser: builds cross-platform binaries (linux/darwin/windows × amd64/arm64)
   - Creates GitHub Release with archives and checksums
   - Docs: deploys VitePress site to GitHub Pages
4. Monitor CI: `gh run list --limit 3`

### Phase 7: GitHub Repository metadata

Ensure the repo looks professional and complete:

1. Set repo description: `gh repo edit scott-walker/kraube-api --description "Lightweight Go gateway for Anthropic Messages API via OAuth subscription"`
2. Set homepage: `gh repo edit scott-walker/kraube-api --homepage "https://scott-walker.github.io/kraube-api/"`
3. Ensure topics are set: go, golang, anthropic, claude, oauth, api-gateway, llm
4. Verify README renders correctly on GitHub (brand image, badges, links)
5. Verify "Releases" sidebar shows the new release
6. Verify "Packages" / Go module is accessible

### Phase 8: Post-release verification

1. Wait for CI to complete: `gh run watch`
2. Verify GitHub Release exists: `gh release view vX.Y.Z`
3. Check release assets: all platform archives + checksums attached
4. Verify documentation site is live: `https://scott-walker.github.io/kraube-api/`
5. Verify module proxy: `curl -s "https://proxy.golang.org/github.com/scott-walker/kraube-api/@v/v$(echo X.Y.Z).info"` (replace X.Y.Z)
6. Test local build: `go build -o kraube ./cmd/kraube/ && ./kraube --help`

### Phase 9: Report

Print a release summary:
- Version released
- GitHub Release URL: `https://github.com/scott-walker/kraube-api/releases/tag/vX.Y.Z`
- Documentation URL: `https://scott-walker.github.io/kraube-api/`
- Number of changes (commits since last tag)
- Breaking changes (if any)
- Install command: `go get github.com/scott-walker/kraube-api@vX.Y.Z`
- Next steps (if any manual steps remain)
