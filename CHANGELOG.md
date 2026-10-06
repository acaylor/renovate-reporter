# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-10-05

### Added

- Run summary with a breakdown by update type (major, minor, patch, other), up to date, and skipped. Each segment works as a filter.
- Comparison with the previous run, or any chosen run, showing newly outdated dependencies and dependencies brought up to date.
- Grouping by repository, with collapsible groups ordered by pending update severity.
- Repository and manager filters, multi-word search, and filter state kept in the URL.
- Dependency details panel with all available updates, release dates, breaking-change flags, skip reasons, warnings, deprecation notices, and source, release, and changelog links.
- Keyboard shortcuts for search, row navigation, and the details panel.
- Dark theme, and a card layout for small screens.
- `/api/deps` rows now include `updateType`, `updates`, `depType`, `skipReason`, `warnings`, `deprecationMessage`, `sourceUrl`, `homepage`, `changelogUrl`, and `currentVersionTimestamp`.
- CSV exports include Update Type, Skip Reason, and Source URL columns.
- Demo Renovate logs in `testdata/demo`.

### Changed

- Export CSV in the UI now exports the rows currently shown instead of the whole log.
- Updated the Go Docker build image from `golang:1.26-alpine` to `golang:1.27-alpine` as suggested by Renovate.
- Updated GitHub Actions dependencies suggested by Renovate: `actions/checkout` from v4 to v7 and `actions/setup-go` from v5 to v7 in the CI and release workflows.
- Updated Docker GitHub Actions dependencies suggested by Renovate: `docker/setup-buildx-action` from v3 to v4, `docker/metadata-action` from v5 to v6, `docker/login-action` from v3 to v4, and `docker/build-push-action` from v6 to v7.

### Fixed

- Log files that change after they are first loaded, such as a log that was still being written, are now reparsed. Deleted log files are removed from the run list.

## [0.1.0] - 2026-05-27

### Added

- Initial Renovate Reporter web UI for browsing dependency data extracted from Renovate debug logs.
- CLI entry point with `renovate-reporter [--port N] <logs-dir>`.
- In-memory parsing and caching for newline-delimited Renovate JSON logs.
- Searchable and sortable dependency table with outdated dependency highlighting.
- CSV export for selected log files.
- Polling for newly added log files.
- Minimal distroless container image published to GitHub Container Registry.
- GitHub Actions CI for tests, CLI builds, and container image publishing.

[Unreleased]: https://github.com/acaylor/renovate-reporter/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/acaylor/renovate-reporter/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/acaylor/renovate-reporter/releases/tag/v0.1.0
