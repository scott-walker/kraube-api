---
name: Release
about: Release checklist for a new Kraube API version
title: "Release vX.Y.Z"
labels: release
assignees: scott-walker
---

## Release vX.Y.Z

### Phase 1 — Pre-release QA

- [ ] All CI checks pass on `main` (lint, vet, test, race)
- [ ] Run full test suite locally: `go test ./... -race -count=1`
- [ ] Test CLI binary manually:
  - [ ] `kraube login` completes OAuth flow
  - [ ] `kraube "test prompt"` returns response
  - [ ] `kraube stream "test"` streams correctly
  - [ ] `kraube usage` shows rate limits
- [ ] Branding check: `grep -r "kraube-go" .` returns zero matches
- [ ] Review CHANGELOG / prepare release notes draft
- [ ] Confirm no TODO/FIXME items block the release

### Phase 2 — Version Bump

- [ ] Update version references in `README.md` if needed
- [ ] Update `CHANGELOG.md` with release notes
- [ ] Commit: `git commit -m "chore: bump version to vX.Y.Z"`
- [ ] Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`

### Phase 3 — Build & Publish

- [ ] Tag push triggers release workflow
- [ ] Monitor GitHub Actions:
  - [ ] Test job passes
  - [ ] GoReleaser job completes (binaries, archives, checksums)
- [ ] Verify GitHub Release page:
  - [ ] Release notes accurate
  - [ ] All platform archives attached (linux/darwin/windows × amd64/arm64)
  - [ ] Checksum file present

### Phase 4 — Documentation

- [ ] Update `docs/` if API surface changed
- [ ] Verify README badges show new version
- [ ] Update `docs/protocol.md` if protocol changed

### Phase 5 — Branding & Presentation

- [ ] Final branding audit: no "kraube-go" references
- [ ] All user-facing strings use "Kraube API" consistently
- [ ] CLI help text and error messages correct
- [ ] GitHub repo description and topics current

### Post-release Verification

- [ ] Install from GitHub Release binary on Linux/macOS
- [ ] `kraube login` + `kraube "test"` works end-to-end
- [ ] `go get github.com/scott-walker/kraube-api@vX.Y.Z` succeeds
- [ ] Monitor GitHub Issues for 48 hours
