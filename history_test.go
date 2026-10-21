package main

import (
	"testing"
	"time"
)

func TestHistoryBackfillAndAppend(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	base := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	for i, rows := range []int{3, 5} {
		result := Result{Columns: []string{"n"}}
		for j := 0; j < rows; j++ {
			result.Rows = append(result.Rows, []string{"x"})
		}
		if _, err := saveResultCSV("q", result, base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	current := base.Add(2 * time.Hour)
	saveResultCSV("q", Result{Columns: []string{"n"}, Rows: [][]string{{"x"}, {"y"}}}, current)
	if err := appendHistory("q", countPoint{At: current, Rows: 2}); err != nil {
		t.Fatal(err)
	}
	points := loadHistory("q")
	if len(points) != 3 {
		t.Fatalf("puntos = %d, quería 3 (relleno de 2 CSV + la ejecución actual sin duplicar): %+v", len(points), points)
	}
	if points[0].Rows != 3 || points[1].Rows != 5 || points[2].Rows != 2 {
		t.Fatalf("filas = %d %d %d", points[0].Rows, points[1].Rows, points[2].Rows)
	}
	appendHistory("q", countPoint{At: current.Add(time.Hour), Error: true})
	points = loadHistory("q")
	if len(points) != 4 || !points[3].Error {
		t.Fatalf("el error debe registrarse como punto: %+v", points)
	}
}
