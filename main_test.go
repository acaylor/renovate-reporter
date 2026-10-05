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

func TestParseLogExtractsTriageFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "renovate.log.json")
	data := `{"repository":"example/repo","config":{"npm":[{"packageFile":"package.json","deps":[` +
		`{"depName":"next","currentValue":"^14.2.3","currentVersion":"14.2.3","depType":"dependencies","sourceUrl":"https://github.com/vercel/next.js","updates":[` +
		`{"newVersion":"14.2.33","updateType":"patch","releaseTimestamp":"2025-09-18T00:00:00Z"},` +
		`{"newVersion":"16.0.4","updateType":"major","isBreaking":true,"releaseTimestamp":"2026-09-30T00:00:00Z"}]},` +
		`{"depName":"internal","currentValue":"workspace:*","skipReason":"unsupported-version"},` +
		`{"depName":"exporter","currentValue":"1.4.0","currentVersion":"1.4.0","warnings":[{"topic":"exporter","message":"Failed to look up"}],"deprecationMessage":"gone"}` +
		`]}]}}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	rows, err := parseLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	byName := map[string]Row{}
	for _, r := range rows {
		byName[r.DepName] = r
	}

	next := byName["next"]
	if next.LatestVersion != "16.0.4" || next.UpdateType != "major" || !next.Outdated {
		t.Fatalf("next: LatestVersion=%q UpdateType=%q Outdated=%v", next.LatestVersion, next.UpdateType, next.Outdated)
	}
	if len(next.Updates) != 2 || !next.Updates[1].IsBreaking || next.Updates[0].UpdateType != "patch" {
		t.Fatalf("next.Updates = %+v", next.Updates)
	}
	if next.SourceURL != "https://github.com/vercel/next.js" || next.DepType != "dependencies" {
		t.Fatalf("next: SourceURL=%q DepType=%q", next.SourceURL, next.DepType)
	}

	internal := byName["internal"]
	if internal.SkipReason != "unsupported-version" || internal.Outdated || internal.UpdateType != "" {
		t.Fatalf("internal: %+v", internal)
	}

	exporter := byName["exporter"]
	if len(exporter.Warnings) != 1 || exporter.Warnings[0] != "Failed to look up" || exporter.DeprecationMessage != "gone" {
		t.Fatalf("exporter: Warnings=%v DeprecationMessage=%q", exporter.Warnings, exporter.DeprecationMessage)
	}
}

func TestSyncDirReloadsChangedAndDropsRemovedFiles(t *testing.T) {
	dir := t.TempDir()
	line := func(dep string) string {
		return `{"repository":"r","config":{"npm":[{"packageFile":"package.json","deps":[{"depName":"` + dep + `","currentValue":"1.0.0"}]}]}}` + "\n"
	}
	a := filepath.Join(dir, "a.log.json")
	b := filepath.Join(dir, "b.log.json")
	if err := os.WriteFile(a, []byte(line("one")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(line("x")), 0o600); err != nil {
		t.Fatal(err)
	}

	cache := newCache()
	known := make(map[string]fileStamp)
	syncDir(dir, cache, known)
	if got := cache.names(); len(got) != 2 || got[0] != "b.log.json" {
		t.Fatalf("names() = %v, want [b.log.json a.log.json]", got)
	}

	// Simulate Renovate still writing a.log.json when it was first loaded.
	if err := os.WriteFile(a, []byte(line("one")+line("two")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	v := cache.ver()
	syncDir(dir, cache, known)

	if rows, _ := cache.rows("a.log.json"); len(rows) != 2 {
		t.Fatalf("a.log.json rows = %d, want 2 after reload", len(rows))
	}
	if _, ok := cache.rows("b.log.json"); ok {
		t.Fatal("b.log.json still cached after removal")
	}
	if got := cache.names(); len(got) != 1 {
		t.Fatalf("names() = %v, want only a.log.json", got)
	}
	if cache.ver() == v {
		t.Fatal("cache version did not change")
	}

	v = cache.ver()
	syncDir(dir, cache, known)
	if cache.ver() != v {
		t.Fatal("unchanged directory bumped the cache version")
	}
}

func TestDemoLogsParse(t *testing.T) {
	for _, name := range []string{"demo-2026-10-04_0600.log.json", "demo-2026-10-05_0600.log.json"} {
		rows, err := parseLog(filepath.Join("testdata", "demo", name))
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatalf("%s: no rows", name)
		}
	}
}
