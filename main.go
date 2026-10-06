package main

import (
	"bufio"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var indexHTML string

// Row holds one extracted dependency from a Renovate log.
type Row struct {
	Repository     string `json:"repository"`
	Manager        string `json:"manager"`
	PackageFile    string `json:"packageFile"`
	DepName        string `json:"depName"`
	PackageName    string `json:"packageName"`
	CurrentValue   string `json:"currentValue"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Datasource     string `json:"datasource"`
	Versioning     string `json:"versioning"`
	Outdated       bool   `json:"outdated"`

	UpdateType              string   `json:"updateType,omitempty"`
	Updates                 []Update `json:"updates,omitempty"`
	DepType                 string   `json:"depType,omitempty"`
	SkipReason              string   `json:"skipReason,omitempty"`
	Warnings                []string `json:"warnings,omitempty"`
	DeprecationMessage      string   `json:"deprecationMessage,omitempty"`
	SourceURL               string   `json:"sourceUrl,omitempty"`
	Homepage                string   `json:"homepage,omitempty"`
	ChangelogURL            string   `json:"changelogUrl,omitempty"`
	CurrentVersionTimestamp string   `json:"currentVersionTimestamp,omitempty"`
}

// Update is one candidate update Renovate proposed for a dependency.
type Update struct {
	NewVersion       string `json:"newVersion"`
	UpdateType       string `json:"updateType,omitempty"`
	ReleaseTimestamp string `json:"releaseTimestamp,omitempty"`
	IsBreaking       bool   `json:"isBreaking,omitempty"`
}

// Cache holds parsed rows keyed by log filename; safe for concurrent use.
type Cache struct {
	mu      sync.RWMutex
	entries map[string][]Row
	sorted  []string // filenames sorted newest-first
	version int
}

func newCache() *Cache {
	return &Cache{entries: make(map[string][]Row)}
}

func (c *Cache) set(name string, rows []Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = rows
	keys := make([]string, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	c.sorted = keys
	c.version++
}

func (c *Cache) remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[name]; !ok {
		return
	}
	delete(c.entries, name)
	keys := c.sorted[:0]
	for _, k := range c.sorted {
		if k != name {
			keys = append(keys, k)
		}
	}
	c.sorted = keys
	c.version++
}

func (c *Cache) names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.sorted))
	copy(out, c.sorted)
	return out
}

func (c *Cache) rows(name string) ([]Row, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.entries[name]
	return r, ok
}

func (c *Cache) ver() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// latestUpdate returns the update with the newest release timestamp, falling
// back to the last entry when no update carries a timestamp.
func latestUpdate(updates []map[string]any) map[string]any {
	if len(updates) == 0 {
		return nil
	}
	var chosen map[string]any
	var maxTS string
	for _, u := range updates {
		if ts, _ := u["releaseTimestamp"].(string); ts != "" && (maxTS == "" || ts > maxTS) {
			maxTS = ts
			chosen = u
		}
	}
	if chosen == nil {
		chosen = updates[len(updates)-1]
	}
	return chosen
}

func updateVersion(u map[string]any) string {
	if v, _ := u["newVersion"].(string); v != "" {
		return v
	}
	v, _ := u["newValue"].(string)
	return v
}

// pickLatest returns the best "latest version" from a dep's updates list.
func pickLatest(updates []map[string]any) string {
	if u := latestUpdate(updates); u != nil {
		return updateVersion(u)
	}
	return ""
}

func warningMessages(v any) []string {
	var out []string
	for _, w := range castSlice(v) {
		switch w := w.(type) {
		case map[string]any:
			if m, _ := w["message"].(string); m != "" {
				out = append(out, m)
			}
		case string:
			out = append(out, w)
		}
	}
	return out
}

func extractDeps(obj map[string]any, seen map[[5]string]bool) []Row {
	var rows []Row
	repository, _ := obj["repository"].(string)
	config, ok := obj["config"].(map[string]any)
	if !ok {
		return nil
	}
	for manager, entriesRaw := range config {
		entries, ok := entriesRaw.([]any)
		if !ok {
			continue
		}
		for _, eRaw := range entries {
			e, ok := eRaw.(map[string]any)
			if !ok {
				continue
			}
			packageFile, _ := e["packageFile"].(string)
			for _, dRaw := range castSlice(e["deps"]) {
				dep, ok := dRaw.(map[string]any)
				if !ok {
					continue
				}
				depName, _ := dep["depName"].(string)
				packageName, _ := dep["packageName"].(string)
				currentValue, _ := dep["currentValue"].(string)
				currentVersion, _ := dep["currentVersion"].(string)
				datasource, _ := dep["datasource"].(string)
				versioning, _ := dep["versioning"].(string)

				var updates []map[string]any
				var candidates []Update
				for _, u := range castSlice(dep["updates"]) {
					um, ok := u.(map[string]any)
					if !ok {
						continue
					}
					updates = append(updates, um)
					c := Update{NewVersion: updateVersion(um)}
					c.UpdateType, _ = um["updateType"].(string)
					c.ReleaseTimestamp, _ = um["releaseTimestamp"].(string)
					c.IsBreaking, _ = um["isBreaking"].(bool)
					candidates = append(candidates, c)
				}
				chosen := latestUpdate(updates)
				var latest string
				if chosen != nil {
					latest = updateVersion(chosen)
				}
				if latest == "" {
					latest = currentVersion
				}
				if latest == "" {
					latest = currentValue
				}
				outdated := latest != "" && latest != currentVersion && latest != currentValue
				var updateType string
				if outdated {
					updateType, _ = chosen["updateType"].(string)
				}

				key := [5]string{repository, packageFile, depName, currentVersion, currentValue}
				if seen[key] {
					continue
				}
				seen[key] = true

				rows = append(rows, Row{
					Repository:     repository,
					Manager:        manager,
					PackageFile:    packageFile,
					DepName:        depName,
					PackageName:    packageName,
					CurrentValue:   currentValue,
					CurrentVersion: currentVersion,
					LatestVersion:  latest,
					Datasource:     datasource,
					Versioning:     versioning,
					Outdated:       outdated,

					UpdateType:              updateType,
					Updates:                 candidates,
					DepType:                 str(dep["depType"]),
					SkipReason:              str(dep["skipReason"]),
					Warnings:                warningMessages(dep["warnings"]),
					DeprecationMessage:      str(dep["deprecationMessage"]),
					SourceURL:               str(dep["sourceUrl"]),
					Homepage:                str(dep["homepage"]),
					ChangelogURL:            str(dep["changelogUrl"]),
					CurrentVersionTimestamp: str(dep["currentVersionTimestamp"]),
				})
			}
		}
	}
	return rows
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func castSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func parseLog(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[[5]string]bool)
	var rows []Row

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)

	parsedAny := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		parsedAny = true
		rows = append(rows, extractDeps(obj, seen)...)
	}
	if err := sc.Err(); err != nil {
		return rows, err
	}

	// Fallback for single pretty-printed JSON files (e.g. example-renovate-log.json).
	if !parsedAny {
		if _, err := f.Seek(0, 0); err == nil {
			var obj map[string]any
			if err := json.NewDecoder(f).Decode(&obj); err == nil {
				rows = append(rows, extractDeps(obj, seen)...)
			}
		}
	}

	return rows, nil
}

// fileStamp identifies a version of a log file on disk.
type fileStamp struct {
	size    int64
	modTime time.Time
}

// syncDir brings cache in line with the .json files in logsDir: new or
// changed files are (re)parsed and deleted files are dropped. known tracks
// the stamp of each file as of the last sync and is updated in place.
func syncDir(logsDir string, cache *Cache, known map[string]fileStamp) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		log.Printf("readdir %s: %v", logsDir, err)
		return
	}
	present := make(map[string]bool)
	var wg sync.WaitGroup
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		present[name] = true
		stamp := fileStamp{size: info.Size(), modTime: info.ModTime()}
		prev, seen := known[name]
		if seen && prev == stamp {
			continue
		}
		known[name] = stamp
		wg.Add(1)
		go func(n string, reload bool) {
			defer wg.Done()
			rows, err := parseLog(filepath.Join(logsDir, n))
			if err != nil {
				log.Printf("parse %s: %v", n, err)
				return
			}
			cache.set(n, rows)
			verb := "loaded"
			if reload {
				verb = "reloaded"
			}
			log.Printf("%-8s %-50s %d rows", verb, n, len(rows))
		}(name, seen)
	}
	wg.Wait()
	for name := range known {
		if !present[name] {
			delete(known, name)
			cache.remove(name)
			log.Printf("removed  %s", name)
		}
	}
}

// watchDir re-syncs the log directory every interval.
func watchDir(logsDir string, cache *Cache, known map[string]fileStamp, interval time.Duration) {
	for {
		time.Sleep(interval)
		syncDir(logsDir, cache, known)
	}
}

func main() {
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: renovate-reporter [--port N] <logs-dir>")
		os.Exit(1)
	}
	logsDir := args[0]

	if stat, err := os.Stat(logsDir); err != nil || !stat.IsDir() {
		fmt.Fprintf(os.Stderr, "not a directory: %s\n", logsDir)
		os.Exit(1)
	}

	cache := newCache()
	log.Printf("loading logs from %s ...", logsDir)
	known := make(map[string]fileStamp)
	syncDir(logsDir, cache, known)
	log.Printf("%d logs loaded", len(cache.names()))

	go watchDir(logsDir, cache, known, 30*time.Second)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})

	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cache.names())
	})

	mux.HandleFunc("/api/deps", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("log")
		rows, ok := cache.rows(name)
		if !ok {
			http.Error(w, "log not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"version": cache.ver()})
	})

	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("log")
		rows, ok := cache.rows(name)
		if !ok {
			http.Error(w, "log not found", http.StatusNotFound)
			return
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".log")
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, stem))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{
			"Repository", "Manager", "Package File", "Dep Name", "Package Name",
			"Current Value", "Current Version", "Latest Version", "Datasource", "Versioning", "Outdated",
			"Update Type", "Skip Reason", "Source URL",
		})
		for _, row := range rows {
			outdated := "no"
			if row.Outdated {
				outdated = "yes"
			}
			_ = cw.Write([]string{
				row.Repository, row.Manager, row.PackageFile, row.DepName, row.PackageName,
				row.CurrentValue, row.CurrentVersion, row.LatestVersion,
				row.Datasource, row.Versioning, outdated,
				row.UpdateType, row.SkipReason, row.SourceURL,
			})
		}
		cw.Flush()
	})

	addr := "0.0.0.0:" + *port
	log.Printf("listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
