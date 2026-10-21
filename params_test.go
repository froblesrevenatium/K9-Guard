package main

import (
	"testing"
	"time"
)

func mustParams(t *testing.T, specs ...string) []Param {
	t.Helper()
	var params []Param
	for _, spec := range specs {
		p, err := parseParamSpec(spec)
		if err != nil {
			t.Fatalf("parseParamSpec(%q): %v", spec, err)
		}
		params = append(params, p)
	}
	return params
}

func TestRenderSQLTSQLStyle(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name     string
		sql      string
		params   []string
		wantSQL  string
		wantArgs []any
	}{
		{"valor entero", "WHERE m.transactiontype = @MyParametro AND m.transactionsubtype = 0", []string{"@MyParametro = 3"}, "WHERE m.transactiontype = $1 AND m.transactionsubtype = 0", []any{int64(3)}},
		{"esquema por punto", "FROM @schema.mastertransaction AS m", []string{"@schema = ixtul"}, `FROM "ixtul".mastertransaction AS m`, nil},
		{"arroba en cadena", "WHERE email = 'a@b.com' AND x = @p", []string{"@p = 'x'"}, "WHERE email = 'a@b.com' AND x = $1", []any{"x"}},
		{"operador de postgres", "WHERE tags @> '{a}' AND id = @id", []string{"@id = 7"}, "WHERE tags @> '{a}' AND id = $1", []any{int64(7)}},
		{"comentario", "SELECT 1 -- @nada\nWHERE a = @a", []string{"@a = true"}, "SELECT 1 -- @nada\nWHERE a = $1", []any{true}},
		{"reutilizado", "WHERE a = @a OR b = @A", []string{"@a = 1.5"}, "WHERE a = $1 OR b = $1", []any{1.5}},
		{"fecha relativa", "WHERE d > @since", []string{"@since = now-24h"}, "WHERE d > $1", []any{now.Add(-24 * time.Hour)}},
		{"texto entre comillas", "WHERE code = @code", []string{"@code = '3'"}, "WHERE code = $1", []any{"3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := renderSQL(tc.sql, mustParams(t, tc.params...), nil, now)
			if err != nil {
				t.Fatalf("renderSQL: %v", err)
			}
			if sql != tc.wantSQL {
				t.Fatalf("sql = %q, quería %q", sql, tc.wantSQL)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, quería %v", args, tc.wantArgs)
			}
			for i := range args {
				if wantTime, ok := tc.wantArgs[i].(time.Time); ok {
					if !args[i].(time.Time).Equal(wantTime) {
						t.Fatalf("arg %d = %v, quería %v", i, args[i], wantTime)
					}
				} else if args[i] != tc.wantArgs[i] {
					t.Fatalf("arg %d = %#v, quería %#v", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestRenderSQLErrors(t *testing.T) {
	if _, _, err := renderSQL("WHERE a = @noDefinido", nil, nil, time.Now()); err == nil {
		t.Fatal("se esperaba error por parámetro no definido")
	}
	if _, err := parseParamSpec("MyParametro = 3"); err == nil {
		t.Fatal("se esperaba error sin @")
	}
	params := mustParams(t, "@a = 1")
	sql, args, err := renderSQL("WHERE a = @a", params, map[string]string{"@a": "5"}, time.Now())
	if err != nil || sql != "WHERE a = $1" || args[0] != int64(5) {
		t.Fatalf("override: %q %v %v", sql, args, err)
	}
}
