package main

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func TestEscEntry(t *testing.T) {
	test.NewApp()
	closed := 0
	entry := newEscEntry(false, func() { closed++ })
	test.Type(entry, "abc")
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if entry.Text != "ab" || closed != 0 {
		t.Fatalf("texto %q, cierres %d", entry.Text, closed)
	}
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if closed != 1 {
		t.Fatalf("Esc debe cerrar, cierres %d", closed)
	}
}

func TestCopyCellCopiesAndToasts(t *testing.T) {
	a := test.NewApp()
	toast := newToastLayer()
	w := test.NewWindow(toast.layer)
	w.Resize(fyne.NewSize(400, 200))
	cell := newCopyCell()
	cell.onCopy = func(text string) { copyToClipboard(text, toast) }
	w.SetContent(container.NewStack(cell, toast.layer))
	cell.label.SetText("CXV522")
	test.TapSecondary(cell)
	items := w.Canvas().Overlays().Top()
	if items == nil {
		t.Fatal("el clic derecho debe abrir el menú")
	}
	var copyItem fyne.Tappable
	for _, o := range test.LaidOutObjects(items) {
		if tappable, ok := o.(fyne.Tappable); ok && fmt.Sprintf("%T", o) == "*widget.menuItem" {
			copyItem = tappable
		}
	}
	if copyItem == nil {
		t.Fatal("falta la opción Copiar")
	}
	test.Tap(copyItem)
	if got := a.Clipboard().Content(); got != "CXV522" {
		t.Fatalf("portapapeles = %q", got)
	}
	if !toast.text.Visible() || toast.text.Text != "Copiado: CXV522" {
		t.Fatalf("aviso = %q visible=%t", toast.text.Text, toast.text.Visible())
	}
}

func TestNumberRows(t *testing.T) {
	got := numberRows(Result{Columns: []string{"a"}, Rows: [][]string{{"x"}, {"y"}}, TotalRows: 2})
	if got.Columns[0] != "#" || got.Rows[0][0] != "1" || got.Rows[1][0] != "2" || got.Rows[1][1] != "y" || got.TotalRows != 2 {
		t.Fatalf("%+v", got)
	}
	if len(numberRows(Result{}).Columns) != 0 {
		t.Fatal("sin columnas no se numera")
	}
}

func TestTapNumberMarksRow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	w := test.NewWindow(nil)
	defer w.Close()
	w.Resize(fyne.NewSize(500, 260))
	var r Result
	table := newResultTable(&r, nil)
	w.SetContent(table)
	showResult(table, &r, numberRows(Result{Columns: []string{"a"}, Rows: [][]string{{"x"}, {"y"}, {"z"}}}))
	tapCell := func(row, col int) {
		var found *copyCell
		for _, o := range test.LaidOutObjects(table) {
			if c, ok := o.(*copyCell); ok && c.Visible() && c.onTap != nil && c.label.Text == r.Rows[row][col] {
				found = c
				break
			}
		}
		if found == nil {
			t.Fatalf("no se encontró la celda %d,%d", row, col)
		}
		test.Tap(found)
	}
	tapCell(1, 0)
	if r.marked != 2 {
		t.Fatalf("clic en # debe marcar la fila, marked=%d", r.marked)
	}
	tapCell(1, 0)
	if r.marked != 2 {
		t.Fatal("doble clic en # también deja marcada la fila")
	}
	test.TapCanvas(w.Canvas(), fyne.NewPos(80, 60))
	if r.marked != 1 {
		t.Fatalf("clic en cualquier celda debe marcar su fila, marked=%d", r.marked)
	}
}

func TestTruncateDecimals(t *testing.T) {
	cases := map[string]string{
		"-2217.9999999999998":        "-2217.9999",
		"1079.9999999999996":         "1079.9999",
		"0.12345":                    "0.1234",
		"-0.99999":                   "-0.9999",
		"455":                        "455",
		"1.5":                        "1.5",
		"12.3400":                    "12.3400",
		"1e-05":                      "0.0000",
		"1.23456789e+02":             "123.4567",
		"1.2.3":                      "1.2.3",
		"brisas":                     "brisas",
		"2026-10-20 16:52:13.123456": "2026-10-20 16:52:13.123456",
		"NaN":                        "NaN",
	}
	for in, want := range cases {
		if got := truncateDecimals(in); got != want {
			t.Errorf("%q = %q, quería %q", in, got, want)
		}
	}
}

func TestIsNumeric(t *testing.T) {
	for value, want := range map[string]bool{"455": true, "-2217.9999": true, "0.0000": true, "1e-05": true, "": false, "brisas": false, "NaN": false, "Inf": false, "HBX718": false, "2026-10-20": false} {
		if isNumeric(value) != want {
			t.Errorf("isNumeric(%q) != %t", value, want)
		}
	}
}
