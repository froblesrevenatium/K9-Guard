package main

import (
	"encoding/csv"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const keptResultsPerQuery = 50

type csvMeta struct {
	TotalRows int  `json:"totalRows"`
	Truncated bool `json:"truncated"`
}

func metaPath(csvPath string) string {
	return strings.TrimSuffix(csvPath, ".csv") + ".meta.json"
}

func notify(title, message, query string) error {
	if nativeNotify(title, message, query) {
		return nil
	}
	log.Printf("notificaciones: enviando por osascript (%s)", title)
	return exec.Command("osascript",
		"-e", "on run argv",
		"-e", `display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"`,
		"-e", "end run",
		title, message).Run()
}

func listCSVs(name string) []string {
	matches, _ := filepath.Glob(filepath.Join(resultsDir(), name, "*.csv"))
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return matches
}

func latestCSV(name string) string {
	if files := listCSVs(name); len(files) > 0 {
		return files[0]
	}
	return ""
}

func pruneCSVs(name string) {
	files := listCSVs(name)
	for i := keptResultsPerQuery; i < len(files); i++ {
		os.Remove(files[i])
		os.Remove(metaPath(files[i]))
	}
}

func saveResultCSV(name string, result Result, at time.Time) (string, error) {
	dir := filepath.Join(resultsDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, at.Format("20060102-150405.000")+".csv")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	writer.Write(result.Columns)
	writer.WriteAll(result.Rows)
	if err := writer.Error(); err != nil {
		return "", err
	}
	writeJSON(metaPath(path), csvMeta{TotalRows: result.Count(), Truncated: result.Truncated})
	pruneCSVs(name)
	return path, nil
}

func readCSV(path string) (Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return Result{}, err
	}
	if len(records) == 0 {
		return Result{}, nil
	}
	result := Result{Columns: records[0], Rows: records[1:], TotalRows: len(records) - 1}
	var meta csvMeta
	if readJSON(metaPath(path), &meta) == nil && meta.TotalRows > 0 {
		result.TotalRows, result.Truncated = meta.TotalRows, meta.Truncated
	}
	return result, nil
}

func csvLabel(path string) string {
	base := filepath.Base(path)
	for _, layout := range []string{"20060102-150405.000.csv", "20060102-150405.csv"} {
		if t, err := time.ParseInLocation(layout, base, time.Local); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return base
}
