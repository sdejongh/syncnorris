//go:build !nogui

package gui

import (
	"image/color"

	"gioui.org/font/gofont"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Color palette
var (
	colorBg        = color.NRGBA{R: 0xfa, G: 0xfa, B: 0xfa, A: 0xff}
	colorSurface   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	colorPrimary   = color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff}
	colorOnPrimary = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	colorText      = color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}
	colorTextMuted = color.NRGBA{R: 0x6b, G: 0x6b, B: 0x6b, A: 0xff}
	colorBorder    = color.NRGBA{R: 0xda, G: 0xda, B: 0xda, A: 0xff}
	colorSuccess   = color.NRGBA{R: 0x1b, G: 0x8a, B: 0x2f, A: 0xff}
	colorError     = color.NRGBA{R: 0xd9, G: 0x30, B: 0x25, A: 0xff}
	colorWarning   = color.NRGBA{R: 0xe6, G: 0x8a, B: 0x00, A: 0xff}
	colorInfo      = color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff}
	colorSkip      = color.NRGBA{R: 0x9e, G: 0x9e, B: 0x9e, A: 0xff}
	colorLogBg     = color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf5, A: 0xff}
	colorCancel    = color.NRGBA{R: 0x75, G: 0x75, B: 0x75, A: 0xff}
)

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         colorBg,
		Fg:         colorText,
		ContrastBg: colorPrimary,
		ContrastFg: colorOnPrimary,
	}
	th.TextSize = unit.Sp(14)
	return th
}
