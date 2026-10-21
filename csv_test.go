package main

import (
	"testing"
	"time"
)

func TestResultCSVRoundTripAndPrune(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	result := Result{Columns: []string{"code", "amount"}, Rows: [][]string{{"CXV522", "-54"}, {"A,B", "línea\nnueva"}}}
	base := time.Date(2026, 10, 20, 13, 0, 0, 0, time.Local)
	var first string
	for i := 0; i < keptResultsPerQuery+5; i++ {
		path, err := saveResultCSV("prueba", result, base.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("saveResultCSV: %v", err)
		}
		if i == 0 {
			first = path
		}
	}
	files := listCSVs("prueba")
	if len(files) != keptResultsPerQuery {
		t.Fatalf("quedaron %d CSV, quería %d", len(files), keptResultsPerQuery)
	}
	if contains(files, first) {
		t.Fatal("el CSV más viejo debió borrarse")
	}
	if got := csvLabel(files[0]); got != "2026-10-20 13:00:54" {
		t.Fatalf("csvLabel = %q", got)
	}
	read, err := readCSV(latestCSV("prueba"))
	if err != nil {
		t.Fatalf("readCSV: %v", err)
	}
	if len(read.Rows) != 2 || read.Rows[1][0] != "A,B" || read.Rows[1][1] != "línea\nnueva" || read.Columns[1] != "amount" {
		t.Fatalf("readCSV = %#v", read)
	}
}

func TestTruncatedCSVKeepsTotal(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	result := Result{Columns: []string{"n"}, Rows: [][]string{{"1"}, {"2"}}, TotalRows: 10000, Truncated: true}
	path, err := saveResultCSV("tope", result, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	read, err := readCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if read.Count() != 10000 || !read.Truncated || len(read.Rows) != 2 {
		t.Fatalf("readCSV = total %d truncado %t filas %d", read.Count(), read.Truncated, len(read.Rows))
	}
}
