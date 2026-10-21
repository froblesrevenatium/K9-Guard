package main

import (
	"testing"
	"time"
)

func TestSeenClearsAlertUntilNextRun(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	if err := updateState("q", func(s *QueryState) { *s = QueryState{LastRun: time.Now(), LastRows: 5, Triggered: true} }); err != nil {
		t.Fatal(err)
	}
	state, _ := loadState()
	if !state["q"].Alert() {
		t.Fatal("una consulta que supera el umbral debe estar en alerta")
	}
	if err := markSeen("q"); err != nil {
		t.Fatal(err)
	}
	state, _ = loadState()
	if state["q"].Alert() || !state["q"].Triggered {
		t.Fatalf("visto debe quitar la alerta y conservar el resultado: %+v", state["q"])
	}
	if err := updateState("q", func(s *QueryState) { *s = QueryState{LastRun: time.Now(), LastRows: 6, Triggered: true} }); err != nil {
		t.Fatal(err)
	}
	state, _ = loadState()
	if !state["q"].Alert() {
		t.Fatal("una ejecución nueva que supera el umbral debe volver a alertar")
	}
	if err := updateState("e", func(s *QueryState) { *s = QueryState{LastRun: time.Now(), LastError: "falla"} }); err != nil {
		t.Fatal(err)
	}
	markSeen("e")
	state, _ = loadState()
	if state["e"].Alert() {
		t.Fatal("un error visto no debe alertar")
	}
}

func TestClearQueryData(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	saveQueries([]Query{{Name: "q"}, {Name: "otra"}})
	updateState("q", func(s *QueryState) { *s = QueryState{LastRun: time.Now(), LastRows: 3, Triggered: true} })
	updateState("otra", func(s *QueryState) { *s = QueryState{LastRun: time.Now(), LastRows: 1} })
	saveResultCSV("q", Result{Columns: []string{"n"}, Rows: [][]string{{"1"}}}, time.Now())
	appendHistory("q", countPoint{At: time.Now(), Rows: 3})
	if err := clearQueryData("q"); err != nil {
		t.Fatal(err)
	}
	state, _ := loadState()
	if _, ok := state["q"]; ok || state["otra"].LastRows != 1 {
		t.Fatalf("estado = %+v", state)
	}
	if len(listCSVs("q")) != 0 || len(loadHistory("q")) != 0 {
		t.Fatal("debían borrarse CSV e historial")
	}
	if _, err := findQuery("q"); err != nil {
		t.Fatal("la consulta debe conservarse")
	}
}

func TestDuplicateQuery(t *testing.T) {
	t.Setenv("DBWATCH_HOME", t.TempDir())
	saveQueries([]Query{{Name: "q", SQL: "SELECT 1", Schemas: []string{"a"}, Enabled: true}, {Name: "q (copia)"}})
	first, err := duplicateQuery("q")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := duplicateQuery("q")
	if first.Name != "q (copia 2)" || second.Name != "q (copia 3)" {
		t.Fatalf("nombres %q, %q", first.Name, second.Name)
	}
	copied, err := findQuery("q (copia 2)")
	if err != nil || copied.SQL != "SELECT 1" || copied.Schemas[0] != "a" || copied.Enabled {
		t.Fatalf("copia = %+v, %v", copied, err)
	}
	if original, _ := findQuery("q"); !original.Enabled {
		t.Fatal("la original no debe cambiar")
	}
}
