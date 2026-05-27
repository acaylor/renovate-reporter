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

// pickLatest returns the best "latest version" from a dep's updates list.
func pickLatest(updates []map[string]any) string {
	if len(updates) == 0 {
		return ""
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
	if v, _ := chosen["newVersion"].(string); v != "" {
		return v
	}
	v, _ := chosen["newValue"].(string)
	return v
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
				for _, u := range castSlice(dep["updates"]) {
					if um, ok := u.(map[string]any); ok {
						updates = append(updates, um)
					}
				}
				latest := pickLatest(updates)
				if latest == "" {
					latest = currentVersion
				}
				if latest == "" {
					latest = currentValue
				}
				outdated := latest != "" && latest != currentVersion && latest != currentValue

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
				})
			}
		}
	}
	return rows
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

func loadAll(logsDir string, cache *Cache) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		log.Printf("readdir %s: %v", logsDir, err)
		return
	}
	var wg sync.WaitGroup
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			rows, err := parseLog(filepath.Join(logsDir, n))
			if err != nil {
				log.Printf("parse %s: %v", n, err)
				return
			}
			cache.set(n, rows)
			log.Printf("loaded %-50s %d rows", n, len(rows))
		}(name)
	}
	wg.Wait()
}

// watchDir polls for new .json files every 30 s and loads them into cache.
func watchDir(logsDir string, cache *Cache) {
	known := make(map[string]bool)
	for _, n := range cache.names() {
		known[n] = true
	}
	for {
		time.Sleep(30 * time.Second)
		entries, _ := os.ReadDir(logsDir)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || known[name] || !strings.HasSuffix(name, ".json") {
				continue
			}
			known[name] = true
			go func(n string) {
				rows, err := parseLog(filepath.Join(logsDir, n))
				if err != nil {
					log.Printf("parse %s: %v", n, err)
					return
				}
				cache.set(n, rows)
				log.Printf("new log loaded: %s (%d rows)", n, len(rows))
			}(name)
		}
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
	loadAll(logsDir, cache)
	log.Printf("%d logs loaded", len(cache.names()))

	go watchDir(logsDir, cache)

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
			})
		}
		cw.Flush()
	})

	addr := "0.0.0.0:" + *port
	log.Printf("listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
