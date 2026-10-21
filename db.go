package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Result struct {
	Columns   []string
	Rows      [][]string
	TotalRows int
	Truncated bool
	Warnings  []string
	Numbered  bool
	marked    int
}

func (r Result) Count() int {
	if r.TotalRows > len(r.Rows) {
		return r.TotalRows
	}
	return len(r.Rows)
}

var leadingCommentsPattern = regexp.MustCompile(`^(\s+|--[^\n]*\n?|/\*(?s:.*?)\*/)*`)

func ensureReadOnlySQL(sql string) error {
	body := strings.ToLower(leadingCommentsPattern.ReplaceAllString(sql, ""))
	if strings.HasPrefix(body, "select") || strings.HasPrefix(body, "with") {
		return nil
	}
	return fmt.Errorf("solo se permiten consultas SELECT o WITH")
}

func connect(ctx context.Context, c Connection, database, schema string) (*pgx.Conn, func(), error) {
	host, port := c.Host, c.Port
	var tunnel *Tunnel
	if c.SSH != nil {
		var err error
		tunnel, err = openTunnel(c)
		if err != nil {
			return nil, nil, err
		}
		host, port = "127.0.0.1", fmt.Sprint(tunnel.LocalPort)
	}
	if database == "" {
		database = c.Database
	}
	if database == "" {
		database = "postgres"
	}
	config, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=prefer connect_timeout=20", host, port, c.User, database))
	if err != nil {
		tunnel.Close()
		return nil, nil, err
	}
	password, err := keychainPassword(c.UUID)
	if err != nil {
		tunnel.Close()
		return nil, nil, err
	}
	config.Password = password
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	config.RuntimeParams["default_transaction_read_only"] = "on"
	config.RuntimeParams["application_name"] = "dbwatch"
	config.RuntimeParams["statement_timeout"] = "300000"
	if schema != "" {
		config.RuntimeParams["search_path"] = pgx.Identifier{schema}.Sanitize()
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		tunnel.Close()
		return nil, nil, err
	}
	closeAll := func() {
		conn.Close(context.Background())
		tunnel.Close()
	}
	return conn, closeAll, nil
}

const permissionPrefix = "Permisos insuficientes: "

func isPermissionError(message string) bool {
	return strings.HasPrefix(message, permissionPrefix)
}

func classifyError(err error) error {
	if err == nil || isPermissionError(err.Error()) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" {
		return errors.New(permissionPrefix + pgErr.Message)
	}
	return err
}

func runQuery(q Query, overrides map[string]string) (Result, error) {
	result, err := runQueryRaw(q, overrides)
	return result, classifyError(err)
}

func runQueryRaw(q Query, overrides map[string]string) (Result, error) {
	if err := ensureReadOnlySQL(q.SQL); err != nil {
		return Result{}, err
	}
	var sql string
	var args []any
	var err error
	if !q.MultiSchema() {
		sql, args, err = renderSQL(q.SQL, effectiveParams(q), overrides, time.Now())
		if err != nil {
			return Result{}, err
		}
	}
	connection, err := findConnection(q.Connection)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	singleSchema := q.Schema
	if q.MultiSchema() {
		singleSchema = ""
	}
	conn, closeAll, err := connect(ctx, connection, q.Database, singleSchema)
	if err != nil {
		return Result{}, err
	}
	defer closeAll()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(context.Background())

	if q.MultiSchema() {
		return runMultiSchema(ctx, tx, q, overrides)
	}

	if q.Schema != "" {
		var exists, allowed bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1), has_schema_privilege(current_user, (SELECT oid FROM pg_namespace WHERE nspname = $1), 'USAGE') IS TRUE", q.Schema).Scan(&exists, &allowed); err != nil {
			return Result{}, err
		}
		if !exists {
			return Result{}, fmt.Errorf("el esquema %s no existe en %s", q.Schema, q.Database)
		}
		if !allowed {
			return Result{}, errors.New(permissionPrefix + "permission denied for schema " + q.Schema)
		}
	}

	var result Result
	err = readRows(ctx, tx, sql, args, &result, q.RowLimit(), nil)
	return result, err
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func readRows(ctx context.Context, source queryer, sql string, args []any, result *Result, limit int, prefix []string) error {
	rows, err := source.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if len(result.Columns) == 0 {
		if len(prefix) > 0 {
			result.Columns = append(result.Columns, "schema")
		}
		for _, field := range rows.FieldDescriptions() {
			result.Columns = append(result.Columns, field.Name)
		}
	}
	for rows.Next() {
		result.TotalRows++
		if len(result.Rows) >= limit {
			result.Truncated = true
			continue
		}
		raw := rows.RawValues()
		row := make([]string, 0, len(prefix)+len(raw))
		row = append(row, prefix...)
		for _, value := range raw {
			if value != nil {
				row = append(row, string(value))
			} else {
				row = append(row, "")
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return rows.Err()
}

func effectiveParams(q Query) []Param {
	if q.Schema == "" {
		return q.Params
	}
	for _, p := range q.Params {
		if strings.EqualFold(p.Name, "schema") {
			return q.Params
		}
	}
	return append(append([]Param{}, q.Params...), Param{Name: "schema", Type: "text", Value: q.Schema})
}

func listColumn(connection, database, sql string) ([]string, error) {
	result, err := runQuery(Query{Connection: connection, Database: database, SQL: sql}, nil)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			values = append(values, row[0])
		}
	}
	return values, nil
}

func listDatabases(connection string) ([]string, error) {
	return listColumn(connection, "", "SELECT datname FROM pg_database WHERE NOT datistemplate AND datallowconn ORDER BY datname")
}

func listSchemas(connection, database string) ([]string, error) {
	return listColumn(connection, database, "SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg\\_%' AND nspname <> 'information_schema' ORDER BY nspname")
}
