# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/acaylor/renovate-reporter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/acaylor/renovate-reporter/releases/tag/v0.1.0
