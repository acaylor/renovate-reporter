package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPickLatestUsesNewestReleaseTimestamp(t *testing.T) {
	updates := []map[string]any{
		{"newVersion": "1.1.0", "releaseTimestamp": "2026-01-01T00:00:00Z"},
		{"newVersion": "1.3.0", "releaseTimestamp": "2026-03-01T00:00:00Z"},
		{"newVersion": "1.2.0", "releaseTimestamp": "2026-02-01T00:00:00Z"},
	}

	if got := pickLatest(updates); got != "1.3.0" {
		t.Fatalf("pickLatest() = %q, want %q", got, "1.3.0")
	}
}

func TestPickLatestFallsBackToLastUpdate(t *testing.T) {
	updates := []map[string]any{
		{"newValue": "1.1.0"},
		{"newValue": "1.2.0"},
	}

	if got := pickLatest(updates); got != "1.2.0" {
		t.Fatalf("pickLatest() = %q, want %q", got, "1.2.0")
	}
}

func TestParseLogExtractsDependenciesFromNDJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "renovate.log.json")
	data := `{"level":30,"msg":"not relevant"}
{"repository":"example/repo","config":{"dockerfile":[{"packageFile":"Dockerfile","deps":[{"depName":"alpine","packageName":"alpine","currentValue":"3.19","currentVersion":"3.19","datasource":"docker","versioning":"docker","updates":[{"newVersion":"3.20","releaseTimestamp":"2026-01-01T00:00:00Z"}]}]}]}}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	rows, err := parseLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Repository != "example/repo" || row.Manager != "dockerfile" || row.PackageFile != "Dockerfile" || row.DepName != "alpine" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.LatestVersion != "3.20" || !row.Outdated {
		t.Fatalf("LatestVersion=%q Outdated=%v, want 3.20 true", row.LatestVersion, row.Outdated)
	}
}
