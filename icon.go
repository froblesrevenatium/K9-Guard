package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

const appName = "K9 Guard"

//go:embed assets/icon.png
var appIconPNG []byte

//go:embed assets/bug.svg
var bugIconSVG []byte

func bugIcon() fyne.Resource {
	return theme.NewThemedResource(fyne.NewStaticResource("bug.svg", bugIconSVG))
}

func appIcon() fyne.Resource {
	return fyne.NewStaticResource("icon.png", appIconPNG)
}

func trayIcon(fill color.NRGBA) fyne.Resource {
	source, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		return appIcon()
	}
	bounds := source.Bounds()
	tinted := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			luminance := (299*uint32(pixel.R) + 587*uint32(pixel.G) + 114*uint32(pixel.B)) / 1000
			alpha := uint32(pixel.A) * (255 - luminance) / 255
			tinted.SetNRGBA(x, y, color.NRGBA{R: fill.R, G: fill.G, B: fill.B, A: uint8(alpha)})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, cropToContent(tinted)); err != nil {
		return appIcon()
	}
	return fyne.NewStaticResource("tray.png", buffer.Bytes())
}

func cropToContent(img *image.NRGBA) image.Image {
	bounds := img.Bounds()
	minX, minY, maxX, maxY := bounds.Max.X, bounds.Max.Y, bounds.Min.X, bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 0 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x+1), max(maxY, y+1)
			}
		}
	}
	if minX >= maxX || minY >= maxY {
		return img
	}
	side := max(maxX-minX, maxY-minY)
	square := image.NewNRGBA(image.Rect(0, 0, side, side))
	offsetX, offsetY := (side-(maxX-minX))/2, (side-(maxY-minY))/2
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			square.SetNRGBA(x-minX+offsetX, y-minY+offsetY, img.NRGBAAt(x, y))
		}
	}
	return square
}

var warningSignSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">` +
	`<path d="M12 1.8 L23 21.2 Q23.3 22.2 22.2 22.2 L1.8 22.2 Q0.7 22.2 1 21.2 Z" fill="#e1ad01" stroke="#7a5d00" stroke-width="0.9" stroke-linejoin="round"/>` +
	`<rect x="10.7" y="8" width="2.6" height="8" rx="1.1" fill="#2b2200"/>` +
	`<circle cx="12" cy="19" r="1.45" fill="#2b2200"/>` +
	`</svg>`)

func runIcon() fyne.Resource {
	return fyne.NewStaticResource("warning-sign.svg", warningSignSVG)
}

func runIconFor(s QueryState) fyne.Resource {
	if s.LastError != "" || len(s.Warnings) > 0 {
		return runIcon()
	}
	return theme.MediaPlayIcon()
}
