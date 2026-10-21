package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type gui struct {
	app      fyne.App
	win      fyne.Window
	desk     desktop.App
	paused   atomic.Bool
	queries  []Query
	state    map[string]QueryState
	selected string

	list             *widget.List
	deleteZone       *tipButton
	editButton       *tipButton
	seenButton       *tipButton
	queryActions     *fyne.Container
	chart            *countChart
	tabs             *container.AppTabs
	permissionBanner *fyne.Container
	permissionText   *widget.Label
	viewer           *fyne.Container
	welcome          fyne.CanvasObject
	chartSummary     *widget.Label
	tooltip          *tooltipLayer
	toast            *toastLayer
	dragName         string
	dragOver         bool
	title            *widget.Label
	details          *widget.Label
	history          *widget.Select
	files            []string
	status           *widget.Label
	table            *widget.Table
	result           Result
	full             Result
	index            []string
	search           *widget.Entry
	csvPath          string

	dbCache     map[string][]string
	schemaCache map[string][]string

	iconOK    fyne.Resource
	iconAlert fyne.Resource
}

const timestampLayout = "2006-01-02 15:04:05"

func stateSymbol(s QueryState) string {
	switch {
	case s.LastRun.IsZero():
		return ""
	case s.Seen && (s.LastError != "" || s.Triggered):
		return "◉ "
	case s.LastError != "":
		return "✖ "
	case s.Triggered:
		return "⚠ "
	default:
		return "✔ "
	}
}

func queryTitle(q Query, s QueryState) string {
	return stateSymbol(s) + q.Name
}

func querySubtitle(s QueryState) string {
	switch {
	case s.LastRun.IsZero():
		return "sin ejecutar"
	case s.LastError != "":
		text := "error · " + s.LastRun.Format(timestampLayout)
		if s.Seen {
			text += " · visto"
		}
		return text
	default:
		text := fmt.Sprintf("%d filas · %s", s.LastRows, s.LastRun.Format(timestampLayout))
		if s.Seen && s.Triggered {
			text += " · visto"
		}
		return text
	}
}

type rowInfoLayout struct{}

func (l *rowInfoLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	title, subtitle := objects[0].MinSize(), objects[1].MinSize()
	return fyne.NewSize(max(title.Width, subtitle.Width), title.Height+subtitle.Height-theme.Padding())
}

func (l *rowInfoLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	title, subtitle := objects[0].MinSize(), objects[1].MinSize()
	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(size.Width, title.Height))
	objects[1].Move(fyne.NewPos(theme.InnerPadding(), title.Height-theme.Padding()))
	objects[1].Resize(fyne.NewSize(size.Width-theme.InnerPadding(), subtitle.Height))
}

func describeState(q Query, s QueryState) string {
	switch {
	case s.LastRun.IsZero():
		return fmt.Sprintf("%s · sin ejecutar", q.Name)
	case s.LastError != "" && s.Seen:
		return fmt.Sprintf("◉ %s · error visto · %s", q.Name, s.LastRun.Format("02/01 15:04"))
	case s.Triggered && s.Seen:
		return fmt.Sprintf("◉ %s · %d filas · visto · %s", q.Name, s.LastRows, s.LastRun.Format("02/01 15:04"))
	case s.LastError != "":
		return fmt.Sprintf("✖ %s · error · %s", q.Name, s.LastRun.Format("02/01 15:04"))
	case s.Triggered:
		return fmt.Sprintf("⚠ %s · %d filas · %s", q.Name, s.LastRows, s.LastRun.Format("02/01 15:04"))
	default:
		return fmt.Sprintf("✔ %s · %d filas · %s", q.Name, s.LastRows, s.LastRun.Format("02/01 15:04"))
	}
}

func newResultTable(result *Result, onCopy func(text string)) *widget.Table {
	var onCellTapped func(widget.TableCellID)
	table := widget.NewTableWithHeaders(
		func() (int, int) {
			if len(result.Columns) == 0 {
				return 0, 0
			}
			return len(result.Rows), len(result.Columns)
		},
		func() fyne.CanvasObject {
			cell := newCopyCell()
			cell.onCopy = onCopy
			return cell
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*copyCell).label
			text := ""
			if id.Row < len(result.Rows) && id.Col < len(result.Rows[id.Row]) {
				text = truncateDecimals(result.Rows[id.Row][id.Col])
			}
			label.Alignment = numberAlignment(*result, id.Col)
			if isNumeric(text) {
				label.Alignment = fyne.TextAlignTrailing
			}
			cell.(*copyCell).setStyle(result.Numbered && id.Col == 0, result.marked == id.Row+1)
			cell.(*copyCell).onTap = func() { onCellTapped(id) }
			label.SetText(text)
		},
	)
	table.ShowHeaderColumn = false
	onCellTapped = func(id widget.TableCellID) {
		marked := id.Row + 1
		if marked != result.marked {
			result.marked = marked
			table.Refresh()
		}
	}
	table.CreateHeader = func() fyne.CanvasObject {
		cell := newCopyCell()
		cell.label.TextStyle = fyne.TextStyle{Bold: true}
		cell.onCopy = onCopy
		return cell
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		label := cell.(*copyCell).label
		label.Alignment = numberAlignment(*result, id.Col)
		if id.Row < 0 && id.Col >= 0 && id.Col < len(result.Columns) {
			label.SetText(result.Columns[id.Col])
		} else {
			label.SetText("")
		}
	}
	return table
}

const (
	minColumnWidth    = 80
	maxColumnWidth    = 420
	sampledWidthRows  = 200
	columnCharWidth   = 7.6
	columnWidthMargin = 28
)

const shownDecimals = 4

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
}

func truncateDecimals(value string) string {
	if !strings.ContainsAny(value, ".eE") {
		return value
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return value
	}
	if strings.ContainsAny(value, "eE") {
		value = strconv.FormatFloat(number, 'f', -1, 64)
	}
	point := strings.IndexByte(value, '.')
	if point < 0 || len(value)-point-1 <= shownDecimals {
		return value
	}
	return value[:point+1+shownDecimals]
}

func numberAlignment(result Result, col int) fyne.TextAlign {
	if result.Numbered && col == 0 {
		return fyne.TextAlignTrailing
	}
	return fyne.TextAlignLeading
}

func columnWidth(result Result, col int) float32 {
	if result.Numbered && col == 0 {
		digits := 1
		if len(result.Rows) > 0 {
			digits = len(result.Rows[len(result.Rows)-1][0])
		}
		return float32(digits)*columnCharWidth + columnWidthMargin
	}
	longest := len([]rune(result.Columns[col]))
	for i, row := range result.Rows {
		if i == sampledWidthRows {
			break
		}
		if col < len(row) {
			longest = max(longest, len([]rune(truncateDecimals(row[col]))))
		}
	}
	return min(maxColumnWidth, max(minColumnWidth, float32(longest)*columnCharWidth+columnWidthMargin))
}

func numberRows(result Result) Result {
	if len(result.Columns) == 0 {
		return result
	}
	numbered := result
	numbered.Numbered = true
	numbered.Columns = append([]string{"#"}, result.Columns...)
	numbered.Rows = make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		numbered.Rows[i] = append([]string{strconv.Itoa(i + 1)}, row...)
	}
	return numbered
}

func showResult(table *widget.Table, target *Result, result Result) {
	*target = result
	table.StickyColumnCount = 0
	if result.Numbered {
		table.StickyColumnCount = 1
	}
	for col := range result.Columns {
		table.SetColumnWidth(col, columnWidth(result, col))
	}
	table.Refresh()
	table.ScrollToTop()
}

func runGUI(showWindow bool) {
	if err := os.MkdirAll(dataDir(), 0o700); err == nil {
		if file, err := os.OpenFile(filepath.Join(dataDir(), "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log.SetOutput(file)
		}
	}
	log.Printf("%s iniciado", appName)
	g := &gui{
		app:         app.NewWithID("com.dbwatch.app"),
		iconOK:      trayIcon(color.NRGBA{R: 255, G: 255, B: 255, A: 255}),
		iconAlert:   trayIcon(color.NRGBA{R: 235, G: 64, B: 52, A: 255}),
		dbCache:     map[string][]string{},
		schemaCache: map[string][]string{},
	}
	g.app.SetIcon(appIcon())
	g.win = g.app.NewWindow(appName)
	g.win.Resize(fyne.NewSize(1200, 740))
	g.win.SetCloseIntercept(func() { g.win.Hide() })
	g.win.SetContent(g.buildContent())

	if desk, ok := g.app.(desktop.App); ok {
		g.desk = desk
	}
	setupNativeNotifications(func(name string) {
		fyne.Do(func() {
			g.focusQuery(name)
			g.tabs.SelectIndex(0)
		})
	})
	g.reload()

	go func() {
		for {
			if !g.paused.Load() {
				runDue(func(Query, Outcome) { fyne.Do(g.reload) })
			}
			time.Sleep(30 * time.Second)
		}
	}()

	if showWindow {
		g.win.Show()
	}
	g.app.Run()
}

func (g *gui) buildContent() fyne.CanvasObject {
	g.list = widget.NewList(
		func() int { return len(g.queries) },
		func() fyne.CanvasObject {
			run := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), nil)
			active := newToggle()
			title := widget.NewLabel("consulta")
			title.Truncation = fyne.TextTruncateEllipsis
			subtitle := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
			subtitle.TextSize = theme.CaptionTextSize()
			subtitle.TextStyle = fyne.TextStyle{Italic: true}
			info := container.New(&rowInfoLayout{}, title, subtitle)
			controls := container.NewHBox(container.NewCenter(container.NewThemeOverride(run, scaledTheme{theme.DefaultTheme(), 0.75})), horizontalSpacer(6), container.NewCenter(active), horizontalSpacer(10))
			row := newDragRow(container.NewBorder(nil, nil, nil, controls, info))
			row.onDrag = g.dragging
			row.onDragEnd = g.dropped
			row.onTap = g.selectInList
			row.onMenu = g.showQueryMenu
			return row
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(g.queries) {
				return
			}
			q := g.queries[id]
			s := g.state[q.Name]
			row := item.(*dragRow)
			row.name = q.Name
			content := row.content.(*fyne.Container)
			info := content.Objects[0].(*fyne.Container)
			controls := content.Objects[1].(*fyne.Container)
			info.Objects[0].(*widget.Label).SetText(queryTitle(q, s))
			subtitle := info.Objects[1].(*canvas.Text)
			subtitle.Text = querySubtitle(s)
			subtitle.Color = theme.Color(theme.ColorNamePlaceHolder)
			subtitle.Refresh()
			run := controls.Objects[0].(*fyne.Container).Objects[0].(*container.ThemeOverride).Content.(*widget.Button)
			active := controls.Objects[2].(*fyne.Container).Objects[0].(*toggle)
			name := q.Name
			run.SetIcon(runIconFor(s))
			run.OnTapped = func() { g.runSaved(name) }
			active.OnChanged = nil
			active.SetOn(q.Enabled)
			active.OnChanged = func(on bool) {
				setEnabled(name, on)
				g.reload()
			}
		},
	)
	g.list.OnSelected = func(id widget.ListItemID) {
		if id < len(g.queries) {
			g.selectQuery(g.queries[id].Name)
		}
	}

	g.title = widget.NewLabelWithStyle("Elige una consulta", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	g.details = widget.NewLabel("")
	g.details.Wrapping = fyne.TextWrapWord
	g.history = widget.NewSelect(nil, func(label string) {
		for _, path := range g.files {
			if csvLabel(path) == label {
				g.viewCSV(path)
			}
		}
	})
	g.history.PlaceHolder = "Sin resultados"
	g.status = widget.NewLabel("")
	g.status.Wrapping = fyne.TextWrapWord
	g.toast = newToastLayer()
	g.table = newResultTable(&g.result, func(text string) { copyToClipboard(text, g.toast) })
	g.search = widget.NewEntry()
	g.search.SetPlaceHolder("Buscar en el resultado…")
	g.search.OnChanged = func(string) { g.applyFilter() }

	g.editButton = newTipButton("", theme.DocumentCreateIcon(), "Editar", func() {
		if q, err := findQuery(g.selected); err == nil {
			g.openEditor(q, q.Name)
		}
	})
	g.seenButton = newTipButton("", theme.ConfirmIcon(), "Enterado", func() {
		if g.selected != "" {
			g.markSeen(g.selected)
		}
	})
	g.seenButton.Importance = widget.LowImportance
	g.editButton.Importance = widget.LowImportance
	g.queryActions = container.NewHBox(tinted(g.seenButton, seenColor), horizontalSpacer(2), tinted(g.editButton, saveColor), horizontalSpacer(4))
	g.queryActions.Hide()
	newButton := widget.NewButtonWithIcon("Nueva consulta", theme.ContentAddIcon(), func() { g.openEditor(Query{Enabled: true, Threshold: 0}, "") })

	clearButton := widget.NewButtonWithIcon("Limpiar", theme.ContentClearIcon(), g.confirmClear)
	historyRow := container.NewBorder(nil, nil, widget.NewLabel("Resultado"), clearButton, g.history)
	searchRow := container.NewBorder(nil, nil, widget.NewIcon(theme.SearchIcon()), nil, g.search)
	titleRow := container.NewBorder(nil, nil, nil, g.queryActions, g.title)
	resultTab := container.New(layout.NewCustomPaddedLayout(8, 0, 0, 0), container.NewBorder(container.NewVBox(historyRow, searchRow, g.status), nil, nil, nil, container.New(layout.NewCustomPaddedLayout(0, -12, -14, -14), g.table)))
	g.chart = newCountChart()
	g.chartSummary = widget.NewLabel("")
	g.chartSummary.Wrapping = fyne.TextWrapWord
	chartTab := container.New(layout.NewCustomPaddedLayout(8, 0, 0, 0), container.NewBorder(g.chartSummary, nil, nil, nil, g.chart))
	g.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Gráfica", theme.GridIcon(), chartTab),
		container.NewTabItemWithIcon("Detalle", theme.ListIcon(), resultTab),
	)
	g.permissionText = widget.NewLabel("")
	g.permissionText.Wrapping = fyne.TextWrapWord
	g.permissionText.TextStyle = fyne.TextStyle{Bold: true}
	permissionBackground := canvas.NewRectangle(permissionColor)
	permissionBackground.CornerRadius = 4
	g.permissionBanner = container.NewStack(permissionBackground, tinted(container.NewPadded(g.permissionText), color.NRGBA{R: 33, G: 26, B: 0, A: 255}))
	g.permissionBanner.Hide()
	viewer := container.NewBorder(container.NewVBox(titleRow, g.details, g.permissionBanner, verticalSpacer(4)), nil, nil, nil, g.tabs)
	g.deleteZone = newTipButton(deleteZoneText, theme.DeleteIcon(), "Arrastra aquí una consulta para eliminarla, o selecciónala y haz clic", func() {
		if g.selected != "" {
			g.confirmRemove(g.selected)
		}
	})
	g.deleteZone.Importance = widget.MediumImportance
	g.tooltip = newTooltipLayer()
	g.deleteZone.onTip = g.tooltip.Show
	g.deleteZone.offTip = g.tooltip.Hide
	g.chart.onHover = g.tooltip.Show
	g.chart.offHover = g.tooltip.Hide
	for _, b := range []*tipButton{g.seenButton, g.editButton} {
		b.onTip = g.tooltip.Show
		b.offTip = g.tooltip.Hide
	}
	deleteHeight := canvas.NewRectangle(color.Transparent)
	deleteHeight.SetMinSize(fyne.NewSize(0, 64))
	deleteArea := container.NewPadded(container.NewStack(deleteHeight, tinted(g.deleteZone, theme.Color(theme.ColorNameError))))
	sourcesButton := newTipButton("", theme.SettingsIcon(), "Fuentes de conexiones", g.openSourcesDialog)
	sourcesButton.Importance = widget.LowImportance
	sourcesButton.onTip = g.tooltip.Show
	sourcesButton.offTip = g.tooltip.Hide
	listHeader := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(sourcesButton, horizontalSpacer(4)), widget.NewLabelWithStyle("Consultas", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		container.NewPadded(tinted(newButton, saveColor)),
	)
	left := container.New(layout.NewCustomPaddedLayout(8, 4, 8, 4), container.NewBorder(listHeader, deleteArea, nil, nil, g.list))
	g.viewer = viewer
	g.welcome = welcomeScreen()
	split := container.NewHSplit(left, container.New(layout.NewCustomPaddedLayout(8, 12, 14, 14), container.NewStack(g.welcome, viewer)))
	viewer.Hide()
	split.Offset = 0.27
	return container.NewStack(split, g.toast.layer, g.tooltip.layer)
}

func (g *gui) reload() {
	queries, err := loadQueries()
	if err != nil {
		g.status.SetText("Error leyendo queries.json: " + err.Error())
	}
	state, _ := loadState()
	g.queries, g.state = queries, state
	g.list.Refresh()
	if g.selected != "" {
		g.refreshSelected()
	}
	g.updateTray()
}

func (g *gui) selectQuery(name string) {
	g.selected = name
	g.refreshSelected()
	if len(g.files) > 0 {
		onChanged := g.history.OnChanged
		g.history.OnChanged = nil
		g.history.SetSelected(csvLabel(g.files[0]))
		g.history.OnChanged = onChanged
		g.viewCSV(g.files[0])
	} else {
		g.csvPath = ""
		g.setViewerData(Result{})
	}
}

func (g *gui) refreshSelected() {
	q, err := findQuery(g.selected)
	if err != nil {
		g.selected = ""
		g.title.SetText("Elige una consulta")
		g.viewer.Hide()
		g.welcome.Show()
		g.details.SetText("")
		g.queryActions.Hide()
		g.permissionBanner.Hide()
		g.chart.SetData(nil, Query{})
		g.chartSummary.SetText("")
		g.history.Options = nil
		g.history.ClearSelected()
		g.history.Refresh()
		g.csvPath = ""
		g.setViewerData(Result{})
		return
	}
	s := g.state[q.Name]
	g.title.SetText(q.Name)
	g.queryActions.Show()
	g.welcome.Hide()
	g.viewer.Show()
	if s.Alert() {
		g.seenButton.Enable()
	} else {
		g.seenButton.Disable()
	}
	origin := q.Connection + " / " + q.Database
	if text := q.SchemaText(); text != "" {
		origin += " / " + text
	}
	g.details.SetText(fmt.Sprintf("%s · %s · notifica si %s · %s", origin, describeSchedule(q), q.RuleText(), map[bool]string{true: "activa", false: "inactiva"}[q.Enabled]))
	if notice := stateNotice(s); notice != "" {
		g.permissionText.SetText(notice)
		g.permissionBanner.Show()
	} else {
		g.permissionBanner.Hide()
	}
	if s.LastError != "" {
		g.status.SetText("Último error: " + s.LastError)
	} else {
		g.status.SetText(describeState(q, s))
	}
	g.updateChart(q)
	g.files = listCSVs(q.Name)
	labels := make([]string, 0, len(g.files))
	for _, path := range g.files {
		labels = append(labels, csvLabel(path))
	}
	selected := g.history.Selected
	onChanged := g.history.OnChanged
	g.history.OnChanged = nil
	g.history.Options = labels
	if selected == "" || !contains(labels, selected) {
		g.history.ClearSelected()
	} else {
		g.history.SetSelected(selected)
	}
	g.history.Refresh()
	g.history.OnChanged = onChanged
}

func (g *gui) viewCSV(path string) {
	result, err := readCSV(path)
	if err != nil {
		g.csvPath = ""
		g.setViewerData(Result{})
		g.status.SetText("No se pudo leer " + path + ": " + err.Error())
		return
	}
	g.csvPath = path
	g.setViewerData(result)
}

func (g *gui) setViewerData(result Result) {
	g.full = numberRows(result)
	g.index = make([]string, len(result.Rows))
	for i, row := range result.Rows {
		g.index[i] = strings.ToLower(strings.Join(row, "\x00"))
	}
	g.applyFilter()
}

func (g *gui) applyFilter() {
	term := strings.ToLower(strings.TrimSpace(g.search.Text))
	shown := g.full
	if term != "" {
		filtered := Result{Columns: g.full.Columns, Numbered: g.full.Numbered}
		for i, row := range g.full.Rows {
			if strings.Contains(g.index[i], term) {
				filtered.Rows = append(filtered.Rows, row)
			}
		}
		shown = filtered
	}
	showResult(g.table, &g.result, shown)
	g.updateViewerStatus(term)
}

func (g *gui) updateViewerStatus(term string) {
	if g.csvPath == "" {
		return
	}
	q, _ := findQuery(g.selected)
	verdict := "no cumple la regla"
	if q.Matches(g.full.Count()) {
		verdict = "cumple la regla"
	}
	parts := []string{csvLabel(g.csvPath), fmt.Sprintf("%d registros", g.full.Count()), verdict}
	if g.full.Truncated {
		parts = append(parts, fmt.Sprintf("truncado: se guardaron las primeras %d filas (tope %d)", len(g.full.Rows), q.RowLimit()))
	}
	if term != "" {
		parts = append(parts, fmt.Sprintf("búsqueda: %d de %d filas", len(g.result.Rows), len(g.full.Rows)))
	}
	g.status.SetText(strings.Join(parts, " · "))
}

func (g *gui) focusQuery(name string) {
	g.showWindow()
	for i, q := range g.queries {
		if q.Name == name {
			g.list.Select(i)
			return
		}
	}
}

func (g *gui) runSaved(name string) {
	q, err := findQuery(name)
	if err != nil {
		g.status.SetText(err.Error())
		return
	}
	if g.selected == name {
		g.status.SetText("Ejecutando " + q.Name + "…")
	}
	go func() {
		outcome := executeExclusive(q, nil, true)
		logOutcome(q, outcome)
		fyne.Do(func() {
			g.reload()
			if g.selected == name {
				if outcome.Err != nil {
					g.status.SetText("Error: " + outcome.Err.Error())
				} else if outcome.CSVPath != "" {
					g.history.SetSelected(csvLabel(outcome.CSVPath))
				}
			}
		})
	}()
}

const deleteZoneText = "Eliminar"

func (g *gui) overDeleteZone(at fyne.Position) bool {
	origin := fyne.CurrentApp().Driver().AbsolutePositionForObject(g.deleteZone)
	size := g.deleteZone.Size()
	return at.X >= origin.X && at.X <= origin.X+size.Width && at.Y >= origin.Y && at.Y <= origin.Y+size.Height
}

func (g *gui) dragging(name string, at fyne.Position) {
	if name == "" {
		return
	}
	over := g.overDeleteZone(at)
	if name == g.dragName && over == g.dragOver {
		return
	}
	g.dragName, g.dragOver = name, over
	g.deleteZone.hideTip()
	if over {
		g.deleteZone.SetText("Suelta para eliminar «" + name + "»")
		g.deleteZone.Importance = widget.DangerImportance
	} else {
		g.deleteZone.SetText("Eliminar «" + name + "»")
		g.deleteZone.Importance = widget.MediumImportance
	}
	g.deleteZone.Refresh()
}

func (g *gui) dropped(name string) {
	over := g.dragOver
	g.dragName, g.dragOver = "", false
	g.deleteZone.SetText(deleteZoneText)
	g.deleteZone.Importance = widget.MediumImportance
	g.deleteZone.Refresh()
	if over && name != "" {
		g.confirmRemove(name)
	}
}

func (g *gui) confirmRemove(name string) {
	dialog.ShowConfirm("Eliminar", "¿Eliminar la consulta "+name+"? Sus CSV se conservan en la carpeta de resultados.", func(ok bool) {
		if !ok {
			return
		}
		queries, err := loadQueries()
		if err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		kept := queries[:0]
		for _, q := range queries {
			if q.Name != name {
				kept = append(kept, q)
			}
		}
		if err := saveQueries(kept); err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		if g.selected == name {
			g.list.UnselectAll()
			g.selected = ""
		}
		g.reload()
		g.refreshSelected()
	}, g.win)
}

func (g *gui) showWindow() {
	g.win.Show()
	g.win.RequestFocus()
}

func (g *gui) updateTray() {
	if g.desk == nil {
		return
	}
	alerts := 0
	items := []*fyne.MenuItem{fyne.NewMenuItem("Abrir "+appName, g.showHome), fyne.NewMenuItemSeparator()}
	for _, q := range g.queries {
		s := g.state[q.Name]
		if s.Alert() {
			alerts++
		}
		name := q.Name
		item := fyne.NewMenuItem(describeState(q, s), nil)
		children := []*fyne.MenuItem{}
		if s.Alert() {
			children = append(children, fyne.NewMenuItem("Enterado", func() { g.markSeen(name) }))
		}
		item.ChildMenu = fyne.NewMenu("", append(children,
			fyne.NewMenuItem("Ejecutar ahora", func() { g.runSaved(name) }),
			fyne.NewMenuItem("Ver resultados", func() { g.focusQuery(name) }),
			fyne.NewMenuItem("Editar", func() {
				g.focusQuery(name)
				if q, err := findQuery(name); err == nil {
					g.openEditor(q, q.Name)
				}
			}),
		)...)
		items = append(items, item)
	}
	if len(g.queries) > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
	}
	pauseLabel := "Pausar horario"
	if g.paused.Load() {
		pauseLabel = "Reanudar horario"
	}
	items = append(items, fyne.NewMenuItem(pauseLabel, func() {
		g.paused.Store(!g.paused.Load())
		g.updateTray()
	}))
	g.desk.SetSystemTrayMenu(fyne.NewMenu(appName, items...))
	if alerts > 0 {
		g.desk.SetSystemTrayIcon(g.iconAlert)
	} else {
		g.desk.SetSystemTrayIcon(g.iconOK)
	}
}

func trimLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func (g *gui) markSeen(name string) {
	if !g.state[name].Alert() {
		if g.selected == name {
			g.status.SetText("Esta consulta no tiene alertas pendientes")
		}
		return
	}
	if err := markSeen(name); err != nil {
		g.status.SetText("No se pudo marcar como visto: " + err.Error())
		return
	}
	g.reload()
}

func (g *gui) updateChart(q Query) {
	points := loadHistory(q.Name)
	g.chart.SetData(points, q)
	if len(points) == 0 {
		g.chartSummary.SetText("Aún no hay ejecuciones registradas para esta consulta.")
		return
	}
	minRows, maxRows, sum, matched, errors := points[0].Rows, points[0].Rows, 0, 0, 0
	for _, point := range points {
		minRows, maxRows = min(minRows, point.Rows), max(maxRows, point.Rows)
		sum += point.Rows
		if point.Error {
			errors++
		} else if q.Matches(point.Rows) {
			matched++
		}
	}
	g.chartSummary.SetText(fmt.Sprintf("%d ejecuciones desde %s · mín %d · máx %d · promedio %.1f · cumplieron la regla %d · errores %d",
		len(points), points[0].At.Format(timestampLayout), minRows, maxRows, float64(sum)/float64(len(points)), matched, errors))
}

var welcomeLines = []string{"Tú eliges dónde mirar;", "yo lo vigilo por ti."}

func welcomeScreen() fyne.CanvasObject {
	foreground := color.NRGBAModel.Convert(theme.Color(theme.ColorNameForeground)).(color.NRGBA)
	logo := canvas.NewImageFromResource(trayIcon(foreground))
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSquareSize(160))
	lines := []fyne.CanvasObject{logo, verticalSpacer(6)}
	for _, text := range welcomeLines {
		line := canvas.NewText(text, theme.Color(theme.ColorNamePlaceHolder))
		line.TextStyle = fyne.TextStyle{Italic: true}
		line.TextSize = theme.TextSize() * 1.3
		line.Alignment = fyne.TextAlignCenter
		lines = append(lines, line)
	}
	return container.NewCenter(container.NewVBox(lines...))
}

func (g *gui) showHome() {
	g.list.UnselectAll()
	g.selected = ""
	g.refreshSelected()
	g.showWindow()
}

func (g *gui) openSourcesDialog() {
	settings := loadSettings()
	sources := discoverSources(settings)
	active := map[string]bool{}
	for _, source := range activeSources() {
		active[source.Dir] = true
	}
	checks := container.NewVBox()
	var rows []struct {
		source connectionSource
		check  *widget.Check
	}
	sshConfigs, sshErr := readSSHConfigs()
	addRow := func(source connectionSource, enabled bool) {
		count := "sin conexiones"
		if found, err := sourceConnections(source, sshConfigs, sshErr); err == nil && len(found) > 0 {
			count = fmt.Sprintf("%d conexiones", len(found))
		}
		check := widget.NewCheck(sourceLabel(source)+"  ·  "+count, nil)
		check.SetChecked(enabled)
		checks.Add(check)
		rows = append(rows, struct {
			source connectionSource
			check  *widget.Check
		}{source, check})
	}
	for _, source := range sources {
		addRow(source, active[source.Dir])
	}
	if len(sources) == 0 {
		checks.Add(widget.NewLabel("No se encontraron proyectos de DataGrip en ~/DataGripProjects."))
	}
	extra := append([]string{}, settings.ExtraProjects...)
	var d dialog.Dialog
	addFolder := widget.NewButtonWithIcon("Agregar carpeta de proyecto…", theme.FolderOpenIcon(), func() {
		picker := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
			if err != nil || folder == nil {
				return
			}
			source, ok := projectSource(folder.Path())
			if !ok {
				dialog.ShowInformation("Fuentes de conexiones", "Esa carpeta no tiene un proyecto de DataGrip (.idea/dataSources.xml).", g.win)
				return
			}
			for _, row := range rows {
				if row.source.Dir == source.Dir {
					row.check.SetChecked(true)
					return
				}
			}
			extra = append(extra, folder.Path())
			addRow(source, true)
		}, g.win)
		picker.Show()
	})
	help := widget.NewLabel("Elige de qué proyectos de DataGrip se toman las conexiones. Las contraseñas se leen del llavero de macOS.")
	help.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(help, addFolder, nil, nil, container.NewVScroll(checks))
	d = dialog.NewCustomConfirm("Fuentes de conexiones", "Guardar", "Cancelar", content, func(ok bool) {
		if !ok {
			return
		}
		next := appSettings{Configured: true, ExtraProjects: extra}
		for _, row := range rows {
			if row.check.Checked {
				next.EnabledDirs = append(next.EnabledDirs, row.source.Dir)
			}
		}
		if err := saveSettings(next); err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		g.dbCache, g.schemaCache = map[string][]string{}, map[string][]string{}
		g.reload()
	}, g.win)
	d.Resize(fyne.NewSize(min(760, g.win.Canvas().Size().Width*0.9), 420))
	d.Show()
}

func stateNotice(s QueryState) string {
	var lines []string
	if isPermissionError(s.LastError) || (s.LastError != "" && strings.Contains(s.LastError, "schemas")) {
		lines = append(lines, s.LastError)
	}
	lines = append(lines, s.Warnings...)
	return strings.Join(lines, "\n")
}

func (g *gui) confirmClear() {
	if g.selected == "" {
		return
	}
	name := g.selected
	dialog.ShowConfirm("Limpiar", "¿Borrar los resultados, el historial de la gráfica y el último estado de "+name+"? La consulta y su configuración se conservan.", func(ok bool) {
		if !ok {
			return
		}
		if err := clearQueryData(name); err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		g.reload()
		g.selectQuery(name)
		g.toast.Show("Se limpió " + name)
	}, g.win)
}

func (g *gui) selectInList(name string) {
	for i, q := range g.queries {
		if q.Name == name {
			g.list.Select(i)
			if g.selected != name {
				g.selectQuery(name)
			}
			return
		}
	}
}

func (g *gui) showQueryMenu(name string, at fyne.Position) {
	menu := fyne.NewMenu("", fyne.NewMenuItem("Duplicar", func() { g.duplicate(name) }))
	widget.ShowPopUpMenuAtPosition(menu, g.win.Canvas(), at)
}

func (g *gui) duplicate(name string) {
	duplicate, err := duplicateQuery(name)
	if err != nil {
		dialog.ShowError(err, g.win)
		return
	}
	g.reload()
	g.selectInList(duplicate.Name)
	g.toast.Show("Se duplicó como " + duplicate.Name)
}
