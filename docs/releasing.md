# Releasing Kraube API

## Process

1. Ensure all CI checks pass on `main`
2. Update `CHANGELOG.md` with release notes
3. Commit: `git commit -m "chore: bump version to vX.Y.Z"`
4. Tag: `git tag vX.Y.Z`
5. Push: `git push origin vX.Y.Z`

The tag push triggers the [release workflow](.github/workflows/release.yml):
- Runs tests with race detection
- GoReleaser builds cross-platform binaries
- Creates GitHub Release with archives and checksums

## Platforms

| OS | Arch | Format |
|----|------|--------|
| Linux | amd64, arm64 | tar.gz |
| macOS | amd64, arm64 | tar.gz |
| Windows | amd64 | zip |

## Checklist

Use the [release issue template](../.github/ISSUE_TEMPLATE/release.md) for each release.

## Versioning

Semantic versioning: `MAJOR.MINOR.PATCH`

- **MAJOR**: Breaking API changes (new TokenProvider contract, removed types)
- **MINOR**: New features (new providers, new options, new API coverage)
- **PATCH**: Bug fixes, documentation, protocol updates
