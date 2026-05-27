# Renovate Reporter

Renovate Reporter is a small web UI for browsing dependency data extracted from Renovate debug logs.

Point it at a directory of Renovate `.json` or `.log.json` files, then open the local web UI to search, sort, inspect outdated dependencies, and export rows as CSV.

## Features

- Parses newline-delimited JSON Renovate debug logs.
- Serves a self-contained web UI on a local HTTP port.
- Shows repository, manager, package file, dependency, current version, latest version, datasource, and versioning.
- Highlights dependencies that appear outdated.
- Polls the log directory for new `.json` files every 30 seconds.
- Exports the selected log's dependency rows as CSV.
- Runs as a standalone CLI binary or a minimal container image.

## Install From Source

```sh
go install github.com/acaylor/renovate-reporter@latest
```

## Download A Release

Prebuilt binaries are attached to each GitHub release:

```text
https://github.com/acaylor/renovate-reporter/releases
```

Release tags also publish matching container images. For example, for `v0.1.0`:

```sh
docker run --rm \
  -p 8080:8080 \
  -v "$PWD/logs:/logs:ro" \
  ghcr.io/acaylor/renovate-reporter:v0.1.0
```

## CLI Usage

```sh
renovate-reporter [--port N] <logs-dir>
```

Example:

```sh
renovate-reporter --port 8080 ./logs
```

Then open:

```text
http://localhost:8080
```

## Docker Usage

The container expects Renovate logs to be mounted at `/logs`.

```sh
docker run --rm \
  -p 8080:8080 \
  -v "$PWD/logs:/logs:ro" \
  ghcr.io/acaylor/renovate-reporter:latest
```

Then open:

```text
http://localhost:8080
```

To use a different host port:

```sh
docker run --rm \
  -p 9090:8080 \
  -v "$PWD/logs:/logs:ro" \
  ghcr.io/acaylor/renovate-reporter:latest
```

Then open `http://localhost:9090`.

## Docker Compose Example

```yaml
services:
  renovate-reporter:
    image: ghcr.io/acaylor/renovate-reporter:latest
    ports:
      - "8080:8080"
    volumes:
      - ./logs:/logs:ro
```

## Log Format

Renovate Reporter reads each `.json` file in the log directory. It is designed for Renovate debug logs written as newline-delimited JSON.

It looks for Renovate log entries that contain repository configuration data and extracts dependencies from manager entries with `packageFile` and `deps` fields.

## HTTP Endpoints

The web UI uses these local endpoints:

- `GET /api/logs`
- `GET /api/deps?log=<filename>`
- `GET /api/status`
- `GET /export?log=<filename>`

## Development

Run tests:

```sh
go test ./...
```

Build the CLI:

```sh
go build -o renovate-reporter .
```

Build the container image:

```sh
docker build -t renovate-reporter .
```

## License

MIT
