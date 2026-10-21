package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const listUserSchemasSQL = "SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg\\_%' AND nspname NOT IN ('information_schema', 'public') ORDER BY nspname"

func targetSchemas(ctx context.Context, tx pgx.Tx, q Query) ([]string, error) {
	if !q.AllSchemas {
		schemas := append([]string{}, q.Schemas...)
		sort.Strings(schemas)
		return schemas, nil
	}
	rows, err := tx.Query(ctx, listUserSchemasSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		schemas = append(schemas, name)
	}
	return schemas, rows.Err()
}

func schemaQuery(q Query, schema string) Query {
	single := q
	single.Schema = schema
	single.Schemas, single.AllSchemas = nil, false
	single.Params = nil
	for _, p := range q.Params {
		if !strings.EqualFold(p.Name, "schema") {
			single.Params = append(single.Params, p)
		}
	}
	return single
}

func runMultiSchema(ctx context.Context, tx pgx.Tx, q Query, overrides map[string]string) (Result, error) {
	schemas, err := targetSchemas(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	if len(schemas) == 0 {
		return Result{}, errors.New("no hay schemas para consultar")
	}
	allowed := map[string]bool{}
	exists := map[string]bool{}
	rows, err := tx.Query(ctx, "SELECT n.nspname, has_schema_privilege(current_user, n.oid, 'USAGE') FROM pg_namespace AS n WHERE n.nspname = ANY($1)", schemas)
	if err != nil {
		return Result{}, err
	}
	for rows.Next() {
		var name string
		var usage bool
		if err := rows.Scan(&name, &usage); err != nil {
			rows.Close()
			return Result{}, err
		}
		exists[name], allowed[name] = true, usage
	}
	rows.Close()

	var result Result
	var denied, failed []string
	succeeded := 0
	now := time.Now()
	for _, schema := range schemas {
		switch {
		case !exists[schema]:
			failed = append(failed, schema+": el esquema no existe")
			continue
		case !allowed[schema]:
			denied = append(denied, schema)
			continue
		}
		sql, args, err := renderSQL(q.SQL, effectiveParams(schemaQuery(q, schema)), overrides, now)
		if err != nil {
			return Result{}, err
		}
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return Result{}, err
		}
		_, err = savepoint.Exec(ctx, "SET LOCAL search_path = "+pgx.Identifier{schema}.Sanitize())
		if err == nil {
			err = readRows(ctx, savepoint, sql, args, &result, q.SchemaRowLimit(len(result.Rows)), []string{schema})
		}
		if err != nil {
			savepoint.Rollback(ctx)
			failed = append(failed, schema+": "+classifyError(err).Error())
			continue
		}
		savepoint.Commit(ctx)
		succeeded++
	}

	if len(denied) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%sen %d schemas (%s): permission denied for schema", permissionPrefix, len(denied), summarizeNames(denied, 12)))
	}
	if len(failed) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Errores en %d schemas: %s", len(failed), summarizeList(failed, 5, " · ")))
	}
	if succeeded == 0 {
		return Result{}, errors.New(strings.Join(result.Warnings, " | "))
	}
	return result, nil
}

func summarizeList(values []string, limit int, separator string) string {
	if len(values) <= limit {
		return strings.Join(values, separator)
	}
	return strings.Join(values[:limit], separator) + fmt.Sprintf("%sy %d más", separator, len(values)-limit)
}

func summarizeNames(values []string, limit int) string {
	return summarizeList(values, limit, ", ")
}
