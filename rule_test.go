package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestOperators(t *testing.T) {
	cases := []struct {
		op    string
		count int
		want  bool
	}{
		{">", 6, true}, {">", 5, false},
		{"<", 4, true}, {"<", 5, false},
		{"!=", 4, true}, {"!=", 5, false},
		{"<>", 6, true}, {"<>", 5, false},
		{">=", 5, true}, {">=", 4, false},
		{"<=", 5, true}, {"<=", 6, false},
		{"", 6, true}, {"", 5, false},
	}
	for _, tc := range cases {
		q := Query{Operator: tc.op, Threshold: 5}
		if got := q.Matches(tc.count); got != tc.want {
			t.Errorf("%d %q 5 = %t, quería %t", tc.count, tc.op, got, tc.want)
		}
	}
}

func TestOnChange(t *testing.T) {
	q := Query{Operator: ">", Threshold: 0, OnChange: true}
	previous := QueryState{LastRun: time.Now(), LastRows: 3, Triggered: true}
	if triggered, unchanged := evaluate(q, previous, 3); !triggered || !unchanged {
		t.Fatalf("misma cantidad: triggered=%t unchanged=%t", triggered, unchanged)
	}
	if _, unchanged := evaluate(q, previous, 4); unchanged {
		t.Fatal("cantidad distinta debe notificar")
	}
	if _, unchanged := evaluate(q, QueryState{}, 3); unchanged {
		t.Fatal("la primera ejecución debe notificar")
	}
	if _, unchanged := evaluate(q, QueryState{LastRun: time.Now(), LastError: "falla"}, 3); unchanged {
		t.Fatal("después de un error debe notificar")
	}
	q.OnChange = false
	if _, unchanged := evaluate(q, previous, 3); unchanged {
		t.Fatal("sin la opción siempre notifica")
	}
}

func TestClassifyPermissionError(t *testing.T) {
	err := classifyError(&pgconn.PgError{Code: "42501", Message: "permission denied for table itinerary"})
	if err.Error() != "Permisos insuficientes: permission denied for table itinerary" || !isPermissionError(err.Error()) {
		t.Fatalf("42501 = %q", err)
	}
	other := classifyError(&pgconn.PgError{Code: "42P01", Message: "relation \"x\" does not exist"})
	if isPermissionError(other.Error()) {
		t.Fatalf("otro código no debe ser de permisos: %q", other)
	}
}
