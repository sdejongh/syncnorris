//go:build !nogui

package gui

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// layoutResumeBanner renders a banner only when there is an interrupted job
// (activeJob != nil and runState == statePaused). When the condition is not
// met the method returns zero dimensions so it takes no space.
func (a *appState) layoutResumeBanner(gtx C) D {
	if a.activeJob == nil || a.runState != statePaused {
		return D{}
	}

	// Amber/warning background colour for the banner.
	bannerBg := colorWarning
	bannerBg.A = 0x22

	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return widget.Border{
			Color:        colorWarning,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(6),
		}.Layout(gtx, func(gtx C) D {
			paint.FillShape(gtx.Ops, bannerBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Warning icon placeholder + message text
					layout.Flexed(1, func(gtx C) D {
						lbl := material.Body2(a.theme, "A job was interrupted.")
						lbl.Font.Weight = font.Bold
						lbl.Color = colorWarning
						return lbl.Layout(gtx)
					}),
					layout.Rigid(spacer(8)),
					// Resume button
					layout.Rigid(func(gtx C) D {
						btn := material.Button(a.theme, &a.bannerResumeBtn, "Resume")
						btn.Background = colorSuccess
						btn.CornerRadius = unit.Dp(4)
						btn.Inset = layout.Inset{
							Top: unit.Dp(6), Bottom: unit.Dp(6),
							Left: unit.Dp(16), Right: unit.Dp(16),
						}
						btn.Font.Weight = font.Bold
						return btn.Layout(gtx)
					}),
					layout.Rigid(spacer(6)),
					// Abandon button
					layout.Rigid(func(gtx C) D {
						btn := material.Button(a.theme, &a.bannerAbandonBtn, "Abandon")
						btn.Background = colorCancel
						btn.CornerRadius = unit.Dp(4)
						btn.Inset = layout.Inset{
							Top: unit.Dp(6), Bottom: unit.Dp(6),
							Left: unit.Dp(16), Right: unit.Dp(16),
						}
						return btn.Layout(gtx)
					}),
				)
			})
		})
	})
}
