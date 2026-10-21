package main

import (
	"strings"
	"testing"
	"time"
)

func TestSchemaQueryOverridesSchemaParam(t *testing.T) {
	q := Query{SQL: "SELECT * FROM @schema.itinerary WHERE id > @id", Schemas: []string{"a", "b"}, Params: []Param{{Name: "schema", Value: "otro"}, {Name: "id", Value: "5"}}}
	sql, args, err := renderSQL(q.SQL, effectiveParams(schemaQuery(q, "brisas")), nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if sql != `SELECT * FROM "brisas".itinerary WHERE id > $1` || len(args) != 1 {
		t.Fatalf("sql = %q args = %v", sql, args)
	}
}

func TestSchemaTextAndSummary(t *testing.T) {
	if (Query{AllSchemas: true}).SchemaText() != "todos los schemas" || (Query{Schemas: []string{"a", "b"}}).SchemaText() != "2 schemas" || (Query{Schema: "x"}).SchemaText() != "x" {
		t.Fatal("SchemaText")
	}
	names := []string{"a", "b", "c", "d"}
	if got := summarizeNames(names, 2); got != "a, b, y 2 más" {
		t.Fatalf("summarizeNames = %q", got)
	}
	if !strings.Contains(summarizeList(names, 10, " · "), "d") {
		t.Fatal("summarizeList sin recorte")
	}
}

func TestSchemaRowLimit(t *testing.T) {
	total := Query{MaxRows: 10}
	if total.SchemaRowLimit(7) != 10 {
		t.Fatal("sin la casilla el tope es sobre el total")
	}
	perSchema := Query{MaxRows: 10, PerSchemaMaxRows: true}
	if perSchema.SchemaRowLimit(7) != 17 {
		t.Fatal("con la casilla cada schema aporta hasta el máximo")
	}
}
