package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	noSchema          = "— sin schema —"
	allSchemasOption  = "— todos los schemas —"
	pickSchemasOption = "— elegir varios… —"
)

func multiSchemaLabel(schemas []string) string {
	if len(schemas) == 1 {
		return "1 schema: " + schemas[0]
	}
	return fmt.Sprintf("%d schemas seleccionados", len(schemas))
}

type editor struct {
	g            *gui
	win          fyne.Window
	dlg          *widget.PopUp
	title        string
	tooltip      *tooltipLayer
	available    []string
	multiSchemas []string
	lastSchema   string
	toast        *toastLayer
	previousKey  func(*fyne.KeyEvent)
	body         *fyne.Container
	split        *container.Split
	formScroll   *container.Scroll
	original     string

	name      *escEntry
	conn      *widget.Select
	database  *widget.Select
	schema    *widget.Select
	sql       *escEntry
	params    *escEntry
	every     *escEntry
	threshold *escEntry
	operator  *widget.Select
	onChange  *widget.Check
	maxRows   *escEntry
	perSchema *widget.Check
	rowsLine  *fyne.Container
	enabled   *widget.Check
	status    *widget.Label
	table     *widget.Table
	result    Result
}

func withValue(options []string, value string) []string {
	if value == "" || contains(options, value) {
		return options
	}
	return append([]string{value}, options...)
}

func selectQuietly(sel *widget.Select, options []string, value string) {
	onChanged := sel.OnChanged
	sel.OnChanged = nil
	sel.Options = options
	if value == "" {
		sel.ClearSelected()
	} else {
		sel.SetSelected(value)
	}
	sel.Refresh()
	sel.OnChanged = onChanged
}

func (g *gui) openEditor(q Query, original string) {
	title := "Nueva consulta"
	if original != "" {
		title = "Editar " + original
	}
	g.showWindow()
	e := &editor{g: g, win: g.win, original: original, title: title}
	e.dlg = widget.NewModalPopUp(e.build(), g.win.Canvas())
	e.load(q)
	available := g.win.Canvas().Size()
	e.dlg.Resize(fyne.NewSize(min(1050, available.Width*0.94), min(760, available.Height*0.92)))
	e.previousKey = g.win.Canvas().OnTypedKey()
	g.win.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		if key.Name == fyne.KeyEscape {
			e.close()
		} else if e.previousKey != nil {
			e.previousKey(key)
		}
	})
	e.dlg.Show()
}

func (e *editor) build() fyne.CanvasObject {
	connectionNames := []string{}
	if connections, err := loadConnections(); err == nil {
		for _, c := range connections {
			connectionNames = append(connectionNames, c.Name)
		}
	}
	e.name = newEscEntry(false, e.close)
	e.conn = widget.NewSelect(connectionNames, nil)
	e.conn.PlaceHolder = "Elige una conexión"
	e.database = widget.NewSelect(nil, nil)
	e.database.PlaceHolder = "Elige una base"
	e.schema = widget.NewSelect(nil, nil)
	e.schema.PlaceHolder = noSchema
	e.conn.OnChanged = func(name string) { e.loadDatabases(name, "", "") }
	e.database.OnChanged = func(database string) { e.loadSchemas(e.conn.Selected, database, "") }
	e.schema.OnChanged = func(value string) {
		if value == pickSchemasOption {
			e.pickSchemas()
			return
		}
		e.lastSchema = value
		e.refreshPerSchema()
	}
	e.sql = newEscEntry(true, e.close)
	e.sql.Wrapping = fyne.TextWrapOff
	e.sql.SetMinRowsVisible(12)
	e.sql.TextStyle = fyne.TextStyle{Monospace: true}
	e.params = newEscEntry(true, e.close)
	e.params.SetMinRowsVisible(3)
	e.params.SetPlaceHolder("una por línea: @MyParametro = 3 · @since = now-24h")
	e.every = newEscEntry(false, e.close)
	e.every.SetPlaceHolder("18:30 o 4.5")
	e.threshold = newEscEntry(false, e.close)
	e.operator = widget.NewSelect(operators, nil)
	e.onChange = widget.NewCheck("Solo si hay cambios", nil)
	e.maxRows = newEscEntry(false, e.close)
	e.maxRows.SetPlaceHolder(strconv.Itoa(defaultMaxRows))
	e.perSchema = widget.NewCheck("Máximo por schema", nil)
	e.perSchema.Hide()
	e.rowsLine = container.NewHBox(fixedWidth(e.maxRows, 140), horizontalSpacer(12), e.perSchema)
	e.enabled = widget.NewCheck("Activa", nil)

	form := widget.NewForm(
		widget.NewFormItem("Nombre", container.NewBorder(nil, verticalSpacer(10-theme.Padding()), nil, nil, e.name)),
		widget.NewFormItem("Origen", container.NewGridWithColumns(3,
			container.NewBorder(nil, nil, widget.NewLabel("Conexión"), nil, e.conn),
			container.NewBorder(nil, nil, widget.NewLabel("Base de datos"), nil, e.database),
			container.NewBorder(nil, nil, widget.NewLabel("Schema"), nil, e.schema),
		)),
		widget.NewFormItem("SQL", e.sql),
		widget.NewFormItem("Parámetros", container.NewBorder(nil, verticalSpacer(10-theme.Padding()), nil, nil, e.params)),
		widget.NewFormItem("Horario", container.NewBorder(nil, nil, fixedWidth(e.every, 200), nil)),
		widget.NewFormItem("Notificaciones", container.NewHBox(widget.NewLabel("Si"), e.operator, fixedWidth(e.threshold, 110), horizontalSpacer(12), e.onChange)),
		widget.NewFormItem("Máx. filas", e.rowsLine),
	)
	e.tooltip = newTooltipLayer()
	iconButton := func(icon fyne.Resource, tip string, tapped func()) *tipButton {
		b := newTipButton("", icon, tip, tapped)
		b.onTip = e.tooltip.Show
		b.offTip = e.tooltip.Hide
		return b
	}
	testButton := iconButton(bugIcon(), "Probar sin guardar ni notificar", e.test)
	saveButton := iconButton(theme.DocumentSaveIcon(), "Guardar", e.save)
	cancelButton := iconButton(theme.CancelIcon(), "Cancelar", e.close)
	for _, b := range []*tipButton{testButton, saveButton, cancelButton} {
		b.Importance = widget.LowImportance
	}
	buttons := container.NewHBox(
		tinted(testButton, debugColor),
		horizontalSpacer(2),
		tinted(saveButton, saveColor),
		horizontalSpacer(2),
		tinted(cancelButton, cancelColor),
		horizontalSpacer(4),
	)
	e.status = widget.NewLabel("")
	e.status.Wrapping = fyne.TextWrapWord
	e.status.Hide()
	e.toast = newToastLayer()
	e.table = newResultTable(&e.result, func(text string) { copyToClipboard(text, e.toast) })

	heading := container.NewBorder(verticalSpacer(2), verticalSpacer(2), horizontalSpacer(4), container.NewCenter(buttons), widget.NewLabelWithStyle(e.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	formScroll := container.NewVScroll(form)
	e.split = container.NewVSplit(formScroll, e.table)
	e.split.Offset = 0.62
	e.body = container.NewStack(formScroll)
	e.formScroll = formScroll
	header := container.NewVBox(heading, widget.NewSeparator(), e.status)
	content := container.New(layout.NewCustomPaddedLayout(10, 14, 16, 16), container.NewBorder(header, nil, nil, nil, e.body))
	return container.NewStack(content, e.toast.layer, e.tooltip.layer)
}

func (e *editor) load(q Query) {
	e.name.SetText(q.Name)
	onChanged := e.conn.OnChanged
	e.conn.OnChanged = nil
	if q.Connection == "" {
		e.conn.ClearSelected()
	} else {
		e.conn.SetSelected(q.Connection)
	}
	e.conn.OnChanged = onChanged
	wantSchema := q.Schema
	e.multiSchemas = append([]string{}, q.Schemas...)
	if q.AllSchemas {
		wantSchema = allSchemasOption
	} else if len(q.Schemas) > 0 {
		wantSchema = multiSchemaLabel(q.Schemas)
	}
	e.loadDatabases(q.Connection, q.Database, wantSchema)
	e.sql.SetText(q.SQL)
	lines := make([]string, 0, len(q.Params))
	for _, p := range q.Params {
		lines = append(lines, formatParam(p))
	}
	e.params.SetText(strings.Join(lines, "\n"))
	e.every.SetText(formatSchedule(q))
	e.threshold.SetText(strconv.Itoa(q.Threshold))
	e.operator.SetSelected(q.Op())
	e.onChange.SetChecked(q.OnChange)
	if q.MaxRows > 0 {
		e.maxRows.SetText(strconv.Itoa(q.MaxRows))
	}
	e.perSchema.SetChecked(q.PerSchemaMaxRows)
	e.enabled.SetChecked(q.Enabled)
}

func (e *editor) selectedSchema() string {
	switch selected := e.schema.Selected; {
	case selected == noSchema, selected == allSchemasOption, selected == pickSchemasOption, selected == multiSchemaLabel(e.multiSchemas) && len(e.multiSchemas) > 0:
		return ""
	default:
		return selected
	}
}

func (e *editor) multiSchemaSelected() bool {
	return e.schema.Selected == allSchemasOption || len(e.multiSchemas) > 0 && e.schema.Selected == multiSchemaLabel(e.multiSchemas)
}

func (e *editor) refreshPerSchema() {
	if e.multiSchemaSelected() {
		e.perSchema.Show()
	} else {
		e.perSchema.Hide()
	}
	e.rowsLine.Refresh()
}

func (e *editor) pickSchemas() {
	previous := e.lastSchema
	chosen := map[string]bool{}
	for _, name := range e.multiSchemas {
		chosen[name] = true
	}
	checks := make([]*widget.Check, 0, len(e.available))
	list := container.NewVBox()
	for _, name := range e.available {
		check := widget.NewCheck(name, nil)
		check.SetChecked(chosen[name])
		checks = append(checks, check)
		list.Add(check)
	}
	setAll := func(value bool) {
		for _, check := range checks {
			check.SetChecked(value)
		}
	}
	tools := container.NewHBox(widget.NewButton("Todos", func() { setAll(true) }), widget.NewButton("Ninguno", func() { setAll(false) }))
	content := container.NewBorder(tools, nil, nil, nil, container.NewVScroll(list))
	picker := dialog.NewCustomConfirm("Elegir schemas", "Aceptar", "Cancelar", content, func(ok bool) {
		if !ok {
			selectQuietly(e.schema, e.schema.Options, previous)
			e.refreshPerSchema()
			return
		}
		var selected []string
		for _, check := range checks {
			if check.Checked {
				selected = append(selected, check.Text)
			}
		}
		if len(selected) == 0 {
			e.multiSchemas = nil
			selectQuietly(e.schema, e.schema.Options, noSchema)
			e.lastSchema = noSchema
			e.refreshPerSchema()
			return
		}
		e.multiSchemas = selected
		label := multiSchemaLabel(selected)
		options := []string{}
		for _, option := range e.schema.Options {
			if !strings.HasSuffix(option, "schemas seleccionados") && !strings.HasPrefix(option, "1 schema: ") {
				options = append(options, option)
			}
		}
		selectQuietly(e.schema, withValue(options, label), label)
		e.lastSchema = label
		e.refreshPerSchema()
	}, e.win)
	picker.Resize(fyne.NewSize(420, 520))
	picker.Show()
}

func (e *editor) loadDatabases(connection, wantDatabase, wantSchema string) {
	selectQuietly(e.database, nil, "")
	selectQuietly(e.schema, nil, "")
	if connection == "" {
		return
	}
	apply := func(databases []string) {
		if e.conn.Selected != connection {
			return
		}
		database := wantDatabase
		if database == "" {
			if c, err := findConnection(connection); err == nil && contains(databases, c.Database) {
				database = c.Database
			}
		}
		selectQuietly(e.database, withValue(databases, database), database)
		e.loadSchemas(connection, database, wantSchema)
	}
	if cached, ok := e.g.dbCache[connection]; ok {
		apply(cached)
		return
	}
	e.setStatus("Cargando bases de " + connection + "…")
	go func() {
		databases, err := listDatabases(connection)
		fyne.Do(func() {
			if err != nil {
				e.setStatus("No se pudieron leer las bases: " + err.Error())
				apply(nil)
				return
			}
			e.g.dbCache[connection] = databases
			e.setStatus("")
			apply(databases)
		})
	}()
}

func (e *editor) loadSchemas(connection, database, wantSchema string) {
	selectQuietly(e.schema, nil, "")
	e.refreshPerSchema()
	if connection == "" || database == "" {
		return
	}
	key := connection + "/" + database
	apply := func(schemas []string) {
		if e.conn.Selected != connection || e.database.Selected != database {
			return
		}
		e.available = schemas
		options := append([]string{noSchema, allSchemasOption, pickSchemasOption}, schemas...)
		selected := noSchema
		if wantSchema != "" {
			options = withValue(options, wantSchema)
			selected = wantSchema
		}
		selectQuietly(e.schema, options, selected)
		e.lastSchema = selected
		e.refreshPerSchema()
	}
	if cached, ok := e.g.schemaCache[key]; ok {
		apply(cached)
		return
	}
	e.setStatus("Cargando schemas de " + database + "…")
	go func() {
		schemas, err := listSchemas(connection, database)
		fyne.Do(func() {
			if err != nil {
				e.setStatus("No se pudieron leer los schemas: " + err.Error())
				apply(nil)
				return
			}
			e.g.schemaCache[key] = schemas
			e.setStatus("")
			apply(schemas)
		})
	}()
}

func (e *editor) query() (Query, error) {
	q := Query{
		Name:       strings.TrimSpace(e.name.Text),
		Connection: e.conn.Selected,
		Database:   e.database.Selected,
		Schema:     e.selectedSchema(),
		AllSchemas: e.schema.Selected == allSchemasOption,
		SQL:        e.sql.Text,
		Enabled:    e.enabled.Checked,
	}
	if q.Name == "" || q.Connection == "" {
		return q, errors.New("nombre y conexión son obligatorios")
	}
	if len(e.multiSchemas) > 0 && e.schema.Selected == multiSchemaLabel(e.multiSchemas) {
		q.Schemas = append([]string{}, e.multiSchemas...)
	}
	if err := ensureReadOnlySQL(q.SQL); err != nil {
		return q, err
	}
	for _, line := range trimLines(e.params.Text) {
		p, err := parseParamSpec(line)
		if err != nil {
			return q, err
		}
		q.Params = append(q.Params, p)
	}
	hours, at, err := parseSchedule(e.every.Text)
	if err != nil {
		return q, err
	}
	q.EveryHours, q.At = hours, at
	threshold, err := strconv.Atoi(strings.TrimSpace(e.threshold.Text))
	if err != nil || threshold < 0 {
		return q, errors.New("el umbral debe ser un entero mayor o igual a 0")
	}
	q.Threshold = threshold
	q.Operator = e.operator.Selected
	q.OnChange = e.onChange.Checked
	if text := strings.TrimSpace(e.maxRows.Text); text != "" {
		maxRows, err := strconv.Atoi(text)
		if err != nil || maxRows < 1 {
			return q, errors.New("el máximo de filas debe ser un entero mayor a 0")
		}
		q.MaxRows = maxRows
	}
	q.PerSchemaMaxRows = q.MultiSchema() && e.perSchema.Checked
	if _, _, err := renderSQL(q.SQL, effectiveParams(q), nil, time.Now()); err != nil {
		return q, err
	}
	return q, nil
}

func (e *editor) test() {
	q, err := e.query()
	if err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	e.setStatus("Ejecutando " + q.Name + "…")
	e.showTestTable()
	go func() {
		result, err := runQuery(q, nil)
		fyne.Do(func() {
			if err != nil {
				showResult(e.table, &e.result, Result{})
				e.setStatus("Error: " + err.Error())
				return
			}
			showResult(e.table, &e.result, numberRows(result))
			verdict := "no cumple la regla"
			if q.Matches(result.Count()) {
				verdict = "cumple la regla"
			}
			message := fmt.Sprintf("Prueba sin guardar ni notificar: %d registros, regla %s, %s", result.Count(), q.RuleText(), verdict)
			if result.Truncated {
				message += fmt.Sprintf(" · se muestran las primeras %d (tope %d)", len(result.Rows), q.RowLimit())
			}
			e.setStatus(message)
		})
	}()
}

func (e *editor) save() {
	q, err := e.query()
	if err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	queries, err := loadQueries()
	if err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	kept := make([]Query, 0, len(queries)+1)
	for _, existing := range queries {
		if existing.Name == q.Name && existing.Name != e.original {
			dialog.ShowError(fmt.Errorf("ya existe otra consulta llamada %s", q.Name), e.win)
			return
		}
		if existing.Name == e.original || existing.Name == q.Name {
			continue
		}
		kept = append(kept, existing)
	}
	if err := saveQueries(append(kept, q)); err != nil {
		dialog.ShowError(err, e.win)
		return
	}
	e.close()
	e.g.reload()
	e.g.focusQuery(q.Name)
}

func fixedWidth(object fyne.CanvasObject, width float32) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(width, object.MinSize().Height), object)
}

func (e *editor) showTestTable() {
	if len(e.body.Objects) == 1 && e.body.Objects[0] == e.split {
		return
	}
	e.split.Leading = e.formScroll
	e.body.Objects = []fyne.CanvasObject{e.split}
	e.body.Refresh()
}

func (e *editor) setStatus(text string) {
	e.status.SetText(text)
	if text == "" {
		e.status.Hide()
	} else {
		e.status.Show()
	}
}

func (e *editor) close() {
	e.dlg.Hide()
	e.tooltip.Hide()
	e.g.win.Canvas().SetOnTypedKey(e.previousKey)
}
