package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	chartMarginLeft   = 56
	chartMarginRight  = 18
	chartMarginTop    = 14
	chartMarginBottom = 30
)

type countChart struct {
	widget.BaseWidget
	points    []countPoint
	query     Query
	positions []fyne.Position
	onHover   func(text string, at fyne.Position)
	offHover  func()
}

func newCountChart() *countChart {
	c := &countChart{}
	c.ExtendBaseWidget(c)
	return c
}

func (c *countChart) SetData(points []countPoint, q Query) {
	c.points, c.query = points, q
	c.Refresh()
}

func (c *countChart) MinSize() fyne.Size { return fyne.NewSize(320, 200) }

func (c *countChart) MouseIn(event *desktop.MouseEvent) { c.MouseMoved(event) }

func (c *countChart) MouseMoved(event *desktop.MouseEvent) {
	if len(c.positions) == 0 || c.onHover == nil {
		return
	}
	nearest, best := -1, float32(math.MaxFloat32)
	for i, position := range c.positions {
		if distance := float32(math.Abs(float64(position.X - event.Position.X))); distance < best {
			nearest, best = i, distance
		}
	}
	if nearest < 0 || best > 24 {
		c.MouseOut()
		return
	}
	point := c.points[nearest]
	text := fmt.Sprintf("%s · %d filas", point.At.Format(timestampLayout), point.Rows)
	if point.Error {
		text = point.At.Format(timestampLayout) + " · error"
	}
	c.onHover(text, event.AbsolutePosition)
}

func (c *countChart) MouseOut() {
	if c.offHover != nil {
		c.offHover()
	}
}

func (c *countChart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{chart: c}
	r.rebuild(c.Size())
	return r
}

type chartRenderer struct {
	chart   *countChart
	objects []fyne.CanvasObject
}

func (r *chartRenderer) MinSize() fyne.Size           { return r.chart.MinSize() }
func (r *chartRenderer) Layout(size fyne.Size)        { r.rebuild(size) }
func (r *chartRenderer) Refresh()                     { r.rebuild(r.chart.Size()); canvas.Refresh(r.chart) }
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Destroy()                     {}

func chartText(text string, size float32, col color.Color) *canvas.Text {
	label := canvas.NewText(text, col)
	label.TextSize = size
	return label
}

func (r *chartRenderer) rebuild(size fyne.Size) {
	c := r.chart
	muted := theme.Color(theme.ColorNamePlaceHolder)
	grid := theme.Color(theme.ColorNameSeparator)
	line := theme.Color(theme.ColorNamePrimary)
	small := theme.CaptionTextSize()
	var objects []fyne.CanvasObject
	c.positions = nil

	if len(c.points) == 0 || size.Width < 80 || size.Height < 60 {
		message := chartText("Sin ejecuciones registradas", theme.TextSize(), muted)
		message.Move(fyne.NewPos(chartMarginLeft, size.Height/2-10))
		r.objects = []fyne.CanvasObject{message}
		return
	}

	plotLeft, plotTop := float32(chartMarginLeft), float32(chartMarginTop)
	plotRight, plotBottom := size.Width-chartMarginRight, size.Height-chartMarginBottom
	plotWidth, plotHeight := plotRight-plotLeft, plotBottom-plotTop

	minRows, maxRows := c.points[0].Rows, c.points[0].Rows
	for _, point := range c.points {
		minRows, maxRows = min(minRows, point.Rows), max(maxRows, point.Rows)
	}
	threshold := c.query.Threshold
	minRows, maxRows = min(minRows, threshold), max(maxRows, threshold)
	if minRows > 0 {
		minRows = 0
	}
	if maxRows == minRows {
		maxRows = minRows + 1
	}
	maxRows += max(1, (maxRows-minRows)/10)
	first, last := c.points[0].At, c.points[len(c.points)-1].At
	span := last.Sub(first).Seconds()

	xFor := func(i int) float32 {
		if len(c.points) == 1 || span <= 0 {
			if len(c.points) == 1 {
				return plotLeft + plotWidth/2
			}
			return plotLeft + plotWidth*float32(i)/float32(len(c.points)-1)
		}
		return plotLeft + plotWidth*float32(c.points[i].At.Sub(first).Seconds()/span)
	}
	yFor := func(rows int) float32 {
		return plotBottom - plotHeight*float32(rows-minRows)/float32(maxRows-minRows)
	}

	for _, rows := range []int{minRows, (minRows + maxRows) / 2, maxRows} {
		y := yFor(rows)
		gridLine := canvas.NewLine(grid)
		gridLine.StrokeWidth = 1
		gridLine.Position1, gridLine.Position2 = fyne.NewPos(plotLeft, y), fyne.NewPos(plotRight, y)
		label := chartText(strconv.Itoa(rows), small, muted)
		labelSize := label.MinSize()
		label.Move(fyne.NewPos(plotLeft-labelSize.Width-8, y-labelSize.Height/2))
		objects = append(objects, gridLine, label)
	}

	thresholdY := yFor(threshold)
	thresholdLine := canvas.NewLine(color.NRGBA{R: 255, G: 159, B: 10, A: 200})
	thresholdLine.StrokeWidth = 1.5
	thresholdLine.Position1, thresholdLine.Position2 = fyne.NewPos(plotLeft, thresholdY), fyne.NewPos(plotRight, thresholdY)
	thresholdLabel := chartText("umbral "+c.query.Op()+" "+strconv.Itoa(threshold), small, color.NRGBA{R: 255, G: 159, B: 10, A: 255})
	thresholdLabel.Move(fyne.NewPos(plotRight-thresholdLabel.MinSize().Width, thresholdY-thresholdLabel.MinSize().Height-2))
	objects = append(objects, thresholdLine, thresholdLabel)

	for i := range c.points {
		c.positions = append(c.positions, fyne.NewPos(xFor(i), yFor(c.points[i].Rows)))
	}
	for i := 1; i < len(c.positions); i++ {
		segment := canvas.NewLine(line)
		segment.StrokeWidth = 2
		segment.Position1, segment.Position2 = c.positions[i-1], c.positions[i]
		objects = append(objects, segment)
	}
	radius := float32(3.5)
	if len(c.points) > 120 {
		radius = 2
	}
	for i, position := range c.positions {
		fill := line
		if c.points[i].Error {
			fill = theme.Color(theme.ColorNameError)
		} else if c.query.Matches(c.points[i].Rows) {
			fill = color.NRGBA{R: 255, G: 159, B: 10, A: 255}
		}
		dot := canvas.NewCircle(fill)
		dot.Move(fyne.NewPos(position.X-radius, position.Y-radius))
		dot.Resize(fyne.NewSquareSize(2 * radius))
		objects = append(objects, dot)
	}

	firstLabel := chartText(first.Format(timestampLayout), small, muted)
	firstLabel.Move(fyne.NewPos(plotLeft, plotBottom+8))
	objects = append(objects, firstLabel)
	if len(c.points) > 1 {
		lastLabel := chartText(last.Format(timestampLayout), small, muted)
		lastLabel.Move(fyne.NewPos(plotRight-lastLabel.MinSize().Width, plotBottom+8))
		objects = append(objects, lastLabel)
	}
	r.objects = objects
}
