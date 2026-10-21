package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Param struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Query struct {
	Name             string   `json:"name"`
	Connection       string   `json:"connection"`
	Database         string   `json:"database"`
	Schema           string   `json:"schema"`
	Schemas          []string `json:"schemas,omitempty"`
	AllSchemas       bool     `json:"allSchemas,omitempty"`
	SQL              string   `json:"sql"`
	Params           []Param  `json:"params"`
	EveryHours       float64  `json:"everyHours"`
	At               []string `json:"at,omitempty"`
	Threshold        int      `json:"threshold"`
	Operator         string   `json:"operator"`
	OnChange         bool     `json:"notifyOnChange"`
	MaxRows          int      `json:"maxRows"`
	PerSchemaMaxRows bool     `json:"perSchemaMaxRows,omitempty"`
	Enabled          bool     `json:"enabled"`
}

type QueryState struct {
	LastRun   time.Time `json:"lastRun"`
	LastRows  int       `json:"lastRows"`
	LastError string    `json:"lastError"`
	Triggered bool      `json:"triggered"`
	Notified  bool      `json:"notified"`
	Seen      bool      `json:"seen"`
	Warnings  []string  `json:"warnings,omitempty"`
}

func (s QueryState) Alert() bool {
	return (s.Triggered || s.LastError != "") && !s.Seen
}

func dataDir() string {
	if dir := os.Getenv("DBWATCH_HOME"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dbwatch")
}

func queriesPath() string { return filepath.Join(dataDir(), "queries.json") }
func statePath() string   { return filepath.Join(dataDir(), "state.json") }
func resultsDir() string  { return filepath.Join(dataDir(), "results") }

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadQueries() ([]Query, error) {
	var queries []Query
	err := readJSON(queriesPath(), &queries)
	sort.Slice(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })
	return queries, err
}

func saveQueries(queries []Query) error { return writeJSON(queriesPath(), queries) }

func copyName(name string, taken func(string) bool) string {
	candidate := name + " (copia)"
	for n := 2; taken(candidate); n++ {
		candidate = fmt.Sprintf("%s (copia %d)", name, n)
	}
	return candidate
}

func duplicateQuery(name string) (Query, error) {
	queries, err := loadQueries()
	if err != nil {
		return Query{}, err
	}
	names := map[string]bool{}
	var original *Query
	for i := range queries {
		names[queries[i].Name] = true
		if queries[i].Name == name {
			original = &queries[i]
		}
	}
	if original == nil {
		return Query{}, errors.New("no existe la consulta " + name)
	}
	duplicate := *original
	duplicate.Name = copyName(name, func(candidate string) bool { return names[candidate] })
	duplicate.Schemas = append([]string(nil), original.Schemas...)
	duplicate.Params = append([]Param(nil), original.Params...)
	duplicate.At = append([]string(nil), original.At...)
	duplicate.Enabled = false
	return duplicate, saveQueries(append(queries, duplicate))
}

func findQuery(name string) (Query, error) {
	queries, err := loadQueries()
	if err != nil {
		return Query{}, err
	}
	for _, q := range queries {
		if q.Name == name {
			return q, nil
		}
	}
	return Query{}, errors.New("no existe la consulta " + name)
}

func loadState() (map[string]QueryState, error) {
	state := map[string]QueryState{}
	err := readJSON(statePath(), &state)
	return state, err
}

func saveState(state map[string]QueryState) error { return writeJSON(statePath(), state) }

const defaultMaxRows = 50000

func (q Query) MultiSchema() bool {
	return q.AllSchemas || len(q.Schemas) > 0
}

func (q Query) SchemaText() string {
	switch {
	case q.AllSchemas:
		return "todos los schemas"
	case len(q.Schemas) == 1:
		return q.Schemas[0]
	case len(q.Schemas) > 1:
		return fmt.Sprintf("%d schemas", len(q.Schemas))
	default:
		return q.Schema
	}
}

var operators = []string{">", "<", "!=", "<>", ">=", "<="}

func (q Query) Op() string {
	if contains(operators, q.Operator) {
		return q.Operator
	}
	return ">"
}

func (q Query) Matches(count int) bool {
	switch q.Op() {
	case "<":
		return count < q.Threshold
	case "!=", "<>":
		return count != q.Threshold
	case ">=":
		return count >= q.Threshold
	case "<=":
		return count <= q.Threshold
	default:
		return count > q.Threshold
	}
}

func (q Query) RuleText() string {
	text := fmt.Sprintf("filas %s %d", q.Op(), q.Threshold)
	if q.OnChange {
		text += " y la cantidad cambia"
	}
	return text
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (q Query) RowLimit() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q Query) SchemaRowLimit(accumulated int) int {
	if q.PerSchemaMaxRows {
		return accumulated + q.RowLimit()
	}
	return q.RowLimit()
}

var stateMu sync.Mutex

func updateState(name string, change func(*QueryState)) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	state, err := loadState()
	if err != nil {
		state = map[string]QueryState{}
	}
	current := state[name]
	change(&current)
	state[name] = current
	return saveState(state)
}

func markSeen(name string) error {
	return updateState(name, func(s *QueryState) { s.Seen = true })
}

func setEnabled(name string, enabled bool) error {
	queries, err := loadQueries()
	if err != nil {
		return err
	}
	found := false
	for i := range queries {
		if queries[i].Name == name {
			queries[i].Enabled = enabled
			found = true
		}
	}
	if !found {
		return errors.New("no existe la consulta " + name)
	}
	return saveQueries(queries)
}

func clearQueryData(name string) error {
	stateMu.Lock()
	state, err := loadState()
	if err == nil {
		delete(state, name)
		err = saveState(state)
	}
	stateMu.Unlock()
	if err != nil {
		return err
	}
	historyMu.Lock()
	removeErr := os.Remove(historyPath(name))
	historyMu.Unlock()
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return os.RemoveAll(filepath.Join(resultsDir(), name))
}
