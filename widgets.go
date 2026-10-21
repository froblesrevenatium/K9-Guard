package main

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type toggle struct {
	widget.BaseWidget
	on        bool
	OnChanged func(bool)
}

func newToggle() *toggle {
	t := &toggle{}
	t.ExtendBaseWidget(t)
	return t
}

func (t *toggle) SetOn(on bool) {
	t.on = on
	t.Refresh()
}

func (t *toggle) Tapped(*fyne.PointEvent) {
	t.on = !t.on
	t.Refresh()
	if t.OnChanged != nil {
		t.OnChanged(t.on)
	}
}

func (t *toggle) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (t *toggle) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(color.Transparent)
	knob := canvas.NewCircle(color.White)
	r := &toggleRenderer{t: t, track: track, knob: knob}
	r.Refresh()
	return r
}

type toggleRenderer struct {
	t     *toggle
	track *canvas.Rectangle
	knob  *canvas.Circle
}

func (r *toggleRenderer) MinSize() fyne.Size { return fyne.NewSize(30, 17) }

func (r *toggleRenderer) Layout(size fyne.Size) {
	track := r.MinSize()
	origin := fyne.NewPos((size.Width-track.Width)/2, (size.Height-track.Height)/2)
	r.track.Move(origin)
	r.track.Resize(track)
	r.track.CornerRadius = track.Height / 2
	diameter := track.Height - 4
	x := origin.X + 2
	if r.t.on {
		x = origin.X + track.Width - diameter - 2
	}
	r.knob.Move(fyne.NewPos(x, origin.Y+2))
	r.knob.Resize(fyne.NewSquareSize(diameter))
}

func (r *toggleRenderer) Refresh() {
	if r.t.on {
		r.track.FillColor = color.NRGBA{R: 52, G: 199, B: 89, A: 255}
	} else {
		r.track.FillColor = theme.Color(theme.ColorNameDisabled)
	}
	r.Layout(r.t.Size())
	r.track.Refresh()
	r.knob.Refresh()
}

func (r *toggleRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.knob} }
func (r *toggleRenderer) Destroy()                     {}

type dragRow struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	name      string
	onDrag    func(name string, at fyne.Position)
	onDragEnd func(name string)
	onTap     func(name string)
	onMenu    func(name string, at fyne.Position)
}

func newDragRow(content fyne.CanvasObject) *dragRow {
	d := &dragRow{content: content}
	d.ExtendBaseWidget(d)
	return d
}

func (d *dragRow) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(d.content) }

func (d *dragRow) Dragged(event *fyne.DragEvent) {
	if d.onDrag != nil {
		d.onDrag(d.name, event.AbsolutePosition)
	}
}

func (d *dragRow) Tapped(*fyne.PointEvent) {
	if d.onTap != nil {
		d.onTap(d.name)
	}
}

func (d *dragRow) TappedSecondary(event *fyne.PointEvent) {
	if d.onMenu != nil {
		d.onMenu(d.name, event.AbsolutePosition)
	}
}

func (d *dragRow) DragEnd() {
	if d.onDragEnd != nil {
		d.onDragEnd(d.name)
	}
}

type tipButton struct {
	widget.Button
	tip    string
	onTip  func(text string, at fyne.Position)
	offTip func()
}

func newTipButton(label string, icon fyne.Resource, tip string, tapped func()) *tipButton {
	b := &tipButton{tip: tip}
	b.Text = label
	b.Icon = icon
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *tipButton) MouseIn(event *desktop.MouseEvent) {
	b.Button.MouseIn(event)
	if b.tip != "" && b.onTip != nil {
		b.onTip(b.tip, event.AbsolutePosition)
	}
}

func (b *tipButton) MouseMoved(event *desktop.MouseEvent) {
	b.Button.MouseMoved(event)
	if b.tip != "" && b.onTip != nil {
		b.onTip(b.tip, event.AbsolutePosition)
	}
}

func (b *tipButton) MouseOut() {
	b.Button.MouseOut()
	b.hideTip()
}

func (b *tipButton) hideTip() {
	if b.offTip != nil {
		b.offTip()
	}
}

type tooltipLayer struct {
	layer *fyne.Container
	box   *canvas.Rectangle
	text  *canvas.Text
}

func newTooltipLayer() *tooltipLayer {
	box := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	box.StrokeColor = theme.Color(theme.ColorNameShadow)
	box.StrokeWidth = 1
	box.CornerRadius = 6
	text := canvas.NewText("", theme.Color(theme.ColorNameForeground))
	text.TextSize = theme.TextSize() - 1
	t := &tooltipLayer{layer: container.NewWithoutLayout(box, text), box: box, text: text}
	t.Hide()
	return t
}

func (t *tooltipLayer) Show(message string, at fyne.Position) {
	if origin := fyne.CurrentApp().Driver().AbsolutePositionForObject(t.layer); origin.X != 0 || origin.Y != 0 {
		at = at.Subtract(origin)
	}
	t.text.Text = message
	t.text.Refresh()
	size := t.text.MinSize()
	padding := float32(8)
	boxSize := fyne.NewSize(size.Width+2*padding, size.Height+2*padding)
	limit := t.layer.Size()
	x, y := at.X+14, at.Y+18
	if limit.Width > 0 && x+boxSize.Width > limit.Width {
		x = limit.Width - boxSize.Width - 4
	}
	if limit.Height > 0 && y+boxSize.Height > limit.Height {
		y = at.Y - boxSize.Height - 8
	}
	t.box.Move(fyne.NewPos(x, y))
	t.box.Resize(boxSize)
	t.text.Move(fyne.NewPos(x+padding, y+padding))
	t.text.Resize(size)
	t.box.Show()
	t.text.Show()
	t.layer.Refresh()
}

func (t *tooltipLayer) Hide() {
	t.box.Hide()
	t.text.Hide()
}

type foregroundTheme struct {
	fyne.Theme
	foreground color.Color
}

func (t foregroundTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameForeground && t.foreground != nil {
		return t.foreground
	}
	return t.Theme.Color(name, variant)
}

func tinted(object fyne.CanvasObject, foreground color.Color) fyne.CanvasObject {
	return container.NewThemeOverride(object, foregroundTheme{theme.DefaultTheme(), foreground})
}

var (
	debugColor      = color.NRGBA{R: 245, G: 180, B: 0, A: 255}
	saveColor       = color.NRGBA{R: 10, G: 132, B: 255, A: 255}
	seenColor       = color.NRGBA{R: 52, G: 199, B: 89, A: 255}
	permissionColor = color.NRGBA{R: 225, G: 173, B: 1, A: 255}
	cancelColor     = color.NRGBA{R: 255, G: 69, B: 58, A: 255}
)

type scaledTheme struct {
	fyne.Theme
	factor float32
}

func (t scaledTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding, theme.SizeNameInnerPadding, theme.SizeNameInlineIcon, theme.SizeNameInputRadius:
		return t.Theme.Size(name) * t.factor
	}
	return t.Theme.Size(name)
}

func horizontalSpacer(width float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(width, 0))
	return spacer
}

func verticalSpacer(height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, height))
	return spacer
}

type escEntry struct {
	widget.Entry
	onEscape func()
}

func newEscEntry(multiLine bool, onEscape func()) *escEntry {
	e := &escEntry{onEscape: onEscape}
	e.MultiLine = multiLine
	if multiLine {
		e.Wrapping = fyne.TextWrapWord
	}
	e.ExtendBaseWidget(e)
	return e
}

func (e *escEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && e.onEscape != nil {
		e.onEscape()
		return
	}
	e.Entry.TypedKey(key)
}

type copyCell struct {
	widget.BaseWidget
	label      *widget.Label
	background *canvas.Rectangle
	onCopy     func(text string)
	onTap      func()
}

func newCopyCell() *copyCell {
	c := &copyCell{label: widget.NewLabel(""), background: canvas.NewRectangle(theme.Color(theme.ColorNameHeaderBackground))}
	c.background.Hide()
	c.ExtendBaseWidget(c)
	return c
}

func (c *copyCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(c.background, c.label))
}

func (c *copyCell) setStyle(header, marked bool) {
	c.label.TextStyle = fyne.TextStyle{Bold: header}
	switch {
	case marked:
		c.background.FillColor = theme.Color(theme.ColorNameSelection)
		c.background.Show()
	case header:
		c.background.FillColor = theme.Color(theme.ColorNameHeaderBackground)
		c.background.Show()
	default:
		c.background.Hide()
	}
	c.background.Refresh()
}

func (c *copyCell) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *copyCell) TappedSecondary(e *fyne.PointEvent) {
	if c.onCopy == nil {
		return
	}
	canvas := fyne.CurrentApp().Driver().CanvasForObject(c)
	if canvas == nil {
		return
	}
	text := c.label.Text
	menu := fyne.NewMenu("", fyne.NewMenuItem("Copiar", func() { c.onCopy(text) }))
	widget.ShowPopUpMenuAtPosition(menu, canvas, e.AbsolutePosition)
}

type toastLayer struct {
	layer *fyne.Container
	box   *canvas.Rectangle
	text  *canvas.Text
	seq   int
}

func newToastLayer() *toastLayer {
	box := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	box.StrokeColor = theme.Color(theme.ColorNameSuccess)
	box.StrokeWidth = 1
	box.CornerRadius = 8
	text := canvas.NewText("", theme.Color(theme.ColorNameForeground))
	t := &toastLayer{layer: container.NewWithoutLayout(box, text), box: box, text: text}
	box.Hide()
	text.Hide()
	return t
}

func (t *toastLayer) Show(message string) {
	t.seq++
	current := t.seq
	t.text.Text = message
	t.text.Refresh()
	size := t.text.MinSize()
	padding := fyne.NewSize(16, 10)
	boxSize := fyne.NewSize(size.Width+2*padding.Width, size.Height+2*padding.Height)
	area := t.layer.Size()
	origin := fyne.NewPos((area.Width-boxSize.Width)/2, area.Height-boxSize.Height-24)
	t.box.Move(origin)
	t.box.Resize(boxSize)
	t.text.Move(origin.Add(fyne.NewPos(padding.Width, padding.Height)))
	t.text.Resize(size)
	t.box.Show()
	t.text.Show()
	t.layer.Refresh()
	time.AfterFunc(1800*time.Millisecond, func() {
		fyne.Do(func() {
			if t.seq == current {
				t.box.Hide()
				t.text.Hide()
				t.layer.Refresh()
			}
		})
	})
}

func copyToClipboard(text string, toast *toastLayer) {
	fyne.CurrentApp().Clipboard().SetContent(text)
	if toast == nil {
		return
	}
	preview := []rune(strings.ReplaceAll(text, "\n", " "))
	if len(preview) > 60 {
		preview = append(preview[:60], '…')
	}
	if len(preview) == 0 {
		toast.Show("Copiado: celda vacía")
		return
	}
	toast.Show("Copiado: " + string(preview))
}
