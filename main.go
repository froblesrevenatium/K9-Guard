package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

const usage = `dbwatch (K9 Guard): consultas programadas de solo lectura sobre las conexiones de DataGrip

uso:
  dbwatch connections
  dbwatch add --name N --connection C [--database D] [--schema E] (--sql S | --sql-file F)
              [--every HORAS|HH:MM[,HH:MM]] [--operator OP] [--threshold X] [--on-change] [--max-rows N] [--param "@Nombre = valor" ...] [--disabled] [--replace]
  dbwatch list
  dbwatch show NOMBRE
  dbwatch run NOMBRE [--set "@Nombre=valor" ...] [--notify]
  dbwatch enable NOMBRE | disable NOMBRE | remove NOMBRE
  dbwatch seen NOMBRE
  dbwatch serve
  dbwatch tray
  dbwatch ui
  dbwatch install-agent [--serve] | uninstall-agent
  dbwatch notify-test

parámetros (estilo T-SQL): en el SQL se escriben @Nombre y se definen con @Nombre = valor.
el tipo se deduce del valor: número, true/false, fecha (2026-10-20, now-24h, today-7d) o
'texto' entre comillas simples; si en el SQL va seguido de punto (@schema.tabla) es un esquema.
regla: notifica cuando la cantidad de filas cumple --operator --threshold (por defecto > 0) o cuando falla;
con --on-change solo si la cantidad cambió respecto a la ejecución anterior.
`

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		if executable, err := os.Executable(); err == nil && strings.Contains(executable, ".app/Contents/MacOS/") {
			runGUI(true)
			return
		}
		fmt.Print(usage)
		os.Exit(1)
	}
	command, args := os.Args[1], os.Args[2:]
	switch command {
	case "connections":
		cmdConnections()
	case "add":
		cmdAdd(args)
	case "list":
		cmdList()
	case "show":
		cmdShow(args)
	case "run":
		cmdRun(args)
	case "enable", "disable":
		cmdToggle(args, command == "enable")
	case "remove":
		cmdRemove(args)
	case "seen":
		if err := markSeen(requireName(args)); err != nil {
			fail("%v", err)
		}
	case "serve":
		serve()
	case "tray":
		runGUI(false)
	case "ui":
		runGUI(true)
	case "install-agent":
		cmdInstallAgent(args)
	case "uninstall-agent":
		cmdUninstallAgent()
	case "notify-test":
		if err := notify(appName, "Notificación de prueba", ""); err != nil {
			fail("no se pudo notificar: %v", err)
		}
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
}

func requireName(args []string) string {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fail("falta el nombre de la consulta")
	}
	return args[0]
}

func cmdConnections() {
	connections, err := loadConnections()
	if err != nil {
		fail("%v", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NOMBRE\tFUENTE\tHOST\tBASE\tUSUARIO\tTÚNEL")
	for _, c := range connections {
		tunnel := "no"
		if c.SSH != nil {
			tunnel = c.SSH.User + "@" + c.SSH.Host
		}
		fmt.Fprintf(w, "%s\t%s\t%s:%s\t%s\t%s\t%s\n", c.Name, c.Source, c.Host, c.Port, c.Database, c.User, tunnel)
	}
	w.Flush()
}

func cmdAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	name := fs.String("name", "", "nombre de la consulta")
	connection := fs.String("connection", "", "conexión de DataGrip")
	database := fs.String("database", "", "base de datos (por defecto la de la conexión)")
	schema := fs.String("schema", "", "esquema: se usa como search_path y como @schema")
	schemas := fs.String("schemas", "", "varios esquemas separados por coma, o * para todos; corre la consulta en cada uno y omite los que fallen")
	sql := fs.String("sql", "", "consulta SQL")
	sqlFile := fs.String("sql-file", "", "archivo con la consulta SQL")
	every := fs.String("every", "", "horas entre ejecuciones (6, 0.5) u horas del día (12:30 o \"08:00, 18:30\"); vacío = solo manual")
	threshold := fs.Int("threshold", 0, "notifica si hay más de X registros")
	maxRows := fs.Int("max-rows", 0, "máximo de filas a guardar y mostrar (0 = 50000)")
	operator := fs.String("operator", ">", "comparación con --threshold: > < != <> >= <=")
	onChange := fs.Bool("on-change", false, "solo notificar si la cantidad de filas cambia respecto a la ejecución anterior")
	disabled := fs.Bool("disabled", false, "crearla deshabilitada")
	replace := fs.Bool("replace", false, "reemplazar si ya existe")
	var params multiFlag
	fs.Var(&params, "param", "@Nombre = valor (repetible)")
	fs.Parse(args)

	if *name == "" || *connection == "" {
		fail("--name y --connection son obligatorios")
	}
	if _, err := findConnection(*connection); err != nil {
		fail("%v", err)
	}
	text := *sql
	if *sqlFile != "" {
		raw, err := os.ReadFile(*sqlFile)
		if err != nil {
			fail("%v", err)
		}
		text = string(raw)
	}
	if strings.TrimSpace(text) == "" {
		fail("falta --sql o --sql-file")
	}
	if !contains(operators, *operator) {
		fail("--operator debe ser uno de %s", strings.Join(operators, " "))
	}
	if err := ensureReadOnlySQL(text); err != nil {
		fail("%v", err)
	}
	hours, at, err := parseSchedule(*every)
	if err != nil {
		fail("%v", err)
	}
	q := Query{Name: *name, Connection: *connection, Database: *database, Schema: *schema, SQL: text, EveryHours: hours, At: at, Threshold: *threshold, Operator: *operator, OnChange: *onChange, MaxRows: *maxRows, Enabled: !*disabled}
	if list := strings.TrimSpace(*schemas); list == "*" {
		q.AllSchemas, q.Schema = true, ""
	} else if list != "" {
		for _, part := range strings.Split(list, ",") {
			if part = strings.TrimSpace(part); part != "" {
				q.Schemas = append(q.Schemas, part)
			}
		}
		q.Schema = ""
	}
	for _, spec := range params {
		p, err := parseParamSpec(spec)
		if err != nil {
			fail("%v", err)
		}
		q.Params = append(q.Params, p)
	}
	if _, _, err := renderSQL(q.SQL, effectiveParams(q), nil, time.Now()); err != nil {
		fail("%v", err)
	}

	queries, err := loadQueries()
	if err != nil {
		fail("%v", err)
	}
	kept := queries[:0]
	for _, existing := range queries {
		if existing.Name == q.Name {
			if !*replace {
				fail("ya existe la consulta %s; usa --replace", q.Name)
			}
			continue
		}
		kept = append(kept, existing)
	}
	if err := saveQueries(append(kept, q)); err != nil {
		fail("%v", err)
	}
	fmt.Printf("consulta %s guardada en %s\n", q.Name, queriesPath())
}

func cmdList() {
	queries, err := loadQueries()
	if err != nil {
		fail("%v", err)
	}
	state, _ := loadState()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NOMBRE\tCONEXIÓN\tBASE\tCADA\tREGLA\tACTIVA\tÚLTIMA\tFILAS\tERROR")
	for _, q := range queries {
		s := state[q.Name]
		last := "-"
		if !s.LastRun.IsZero() {
			last = s.LastRun.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%s\t%d\t%s\n", q.Name, q.Connection, q.Database, describeSchedule(q), q.RuleText(), q.Enabled, last, s.LastRows, s.LastError)
	}
	w.Flush()
}

func cmdShow(args []string) {
	q, err := findQuery(requireName(args))
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("nombre:     %s\nconexión:   %s\nbase:       %s\nesquema:    %s\nhorario:    %s\nregla:      %s\nactiva:     %t\n", q.Name, q.Connection, q.Database, q.SchemaText(), describeSchedule(q), q.RuleText(), q.Enabled)
	for _, p := range q.Params {
		fmt.Printf("parámetro:  %s\n", formatParam(p))
	}
	fmt.Printf("sql:\n%s\n", q.SQL)
}

func printResult(result Result, limit int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(result.Columns, "\t"))
	for i, row := range result.Rows {
		if i == limit {
			break
		}
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	if len(result.Rows) > limit {
		fmt.Printf("... %d filas más\n", len(result.Rows)-limit)
	}
}

func cmdRun(args []string) {
	name := requireName(args)
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var sets multiFlag
	fs.Var(&sets, "set", "@Nombre=valor para sobrescribir un parámetro (repetible)")
	sendNotification := fs.Bool("notify", false, "enviar la notificación si se cumple la regla")
	fs.Parse(args[1:])

	q, err := findQuery(name)
	if err != nil {
		fail("%v", err)
	}
	overrides := map[string]string{}
	for _, s := range sets {
		key, value, ok := strings.Cut(s, "=")
		if !ok {
			fail("--set %q: usa @Nombre=valor", s)
		}
		overrides[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	outcome := execute(q, overrides, *sendNotification)
	if outcome.Err != nil {
		fail("error: %v", outcome.Err)
	}
	printResult(outcome.Result, 50)
	for _, warning := range outcome.Result.Warnings {
		fmt.Println("aviso:", warning)
	}
	fmt.Printf("\n%d registros, regla %s: ", outcome.Result.Count(), q.RuleText())
	if outcome.Result.Truncated {
		fmt.Printf("(se guardaron las primeras %d) ", len(outcome.Result.Rows))
	}
	switch {
	case outcome.Triggered && outcome.Unchanged:
		fmt.Printf("cumple la regla, pero la cantidad no cambió (CSV en %s)\n", outcome.CSVPath)
	case outcome.Triggered:
		fmt.Printf("cumple la regla (CSV en %s)\n", outcome.CSVPath)
	default:
		fmt.Println("no cumple la regla")
	}
}

func cmdToggle(args []string, enabled bool) {
	if err := setEnabled(requireName(args), enabled); err != nil {
		fail("%v", err)
	}
}

func cmdRemove(args []string) {
	name := requireName(args)
	queries, err := loadQueries()
	if err != nil {
		fail("%v", err)
	}
	kept := queries[:0]
	for _, q := range queries {
		if q.Name != name {
			kept = append(kept, q)
		}
	}
	if len(kept) == len(queries) {
		fail("no existe la consulta %s", name)
	}
	if err := saveQueries(kept); err != nil {
		fail("%v", err)
	}
}

const agentLabel = "com.dbwatch.serve"

func agentPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

func cmdInstallAgent(args []string) {
	fs := flag.NewFlagSet("install-agent", flag.ExitOnError)
	headless := fs.Bool("serve", false, "correr sin ícono en la barra de menú")
	fs.Parse(args)
	mode := "tray"
	if *headless {
		mode = "serve"
	}
	executable, err := os.Executable()
	if err != nil {
		fail("%v", err)
	}
	executable, _ = filepath.EvalSymlinks(executable)
	logPath := filepath.Join(dataDir(), "serve.log")
	if err := os.MkdirAll(dataDir(), 0o700); err != nil {
		fail("%v", err)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin</string></dict>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, agentLabel, executable, mode, logPath, logPath)
	if err := os.WriteFile(agentPlistPath(), []byte(plist), 0o644); err != nil {
		fail("%v", err)
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	exec.Command("launchctl", "bootout", domain+"/"+agentLabel).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, agentPlistPath()).CombinedOutput(); err != nil {
		fail("launchctl bootstrap: %v %s", err, out)
	}
	fmt.Printf("agente instalado en modo %s (%s); log en %s\n", mode, agentPlistPath(), logPath)
}

func cmdUninstallAgent() {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	exec.Command("launchctl", "bootout", domain+"/"+agentLabel).Run()
	os.Remove(agentPlistPath())
	fmt.Println("agente desinstalado")
}
