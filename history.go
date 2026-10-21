package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const keptHistoryPoints = 5000

type countPoint struct {
	At    time.Time `json:"at"`
	Rows  int       `json:"rows"`
	Error bool      `json:"error,omitempty"`
}

var historyMu sync.Mutex

func historyPath(name string) string {
	return filepath.Join(dataDir(), "history", name+".jsonl")
}

func appendHistory(name string, point countPoint) error {
	historyMu.Lock()
	defer historyMu.Unlock()
	if _, err := os.Stat(historyPath(name)); os.IsNotExist(err) {
		backfillHistoryLocked(name)
	}
	points := readHistoryLocked(name)
	if n := len(points); n > 0 && point.At.Sub(points[n-1].At).Abs() < time.Second {
		points[n-1] = point
	} else {
		points = append(points, point)
	}
	if len(points) > keptHistoryPoints {
		points = points[len(points)-keptHistoryPoints:]
	}
	return writeHistoryLocked(name, points)
}

func loadHistory(name string) []countPoint {
	historyMu.Lock()
	defer historyMu.Unlock()
	if _, err := os.Stat(historyPath(name)); os.IsNotExist(err) {
		backfillHistoryLocked(name)
	}
	return readHistoryLocked(name)
}

func readHistoryLocked(name string) []countPoint {
	file, err := os.Open(historyPath(name))
	if err != nil {
		return nil
	}
	defer file.Close()
	var points []countPoint
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var point countPoint
		if json.Unmarshal(scanner.Bytes(), &point) == nil {
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	return points
}

func writeHistoryLocked(name string, points []countPoint) error {
	path := historyPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var builder strings.Builder
	for _, point := range points {
		line, _ := json.Marshal(point)
		builder.Write(line)
		builder.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(builder.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func csvTime(path string) (time.Time, bool) {
	base := filepath.Base(path)
	for _, layout := range []string{"20060102-150405.000.csv", "20060102-150405.csv"} {
		if t, err := time.ParseInLocation(layout, base, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func backfillHistoryLocked(name string) {
	files := listCSVs(name)
	if len(files) == 0 {
		return
	}
	var points []countPoint
	for _, path := range files {
		at, ok := csvTime(path)
		if !ok {
			continue
		}
		result, err := readCSV(path)
		if err != nil {
			continue
		}
		points = append(points, countPoint{At: at, Rows: result.Count()})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	writeHistoryLocked(name, points)
}
