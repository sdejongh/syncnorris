//go:build !nogui

package gui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

// Panel widths
const (
	configPanelWidth = unit.Dp(640)
	logPanelWidth    = unit.Dp(420)
	separatorWidth   = unit.Dp(25) // 12 + 1 + 12
	windowPadding    = unit.Dp(32) // 16 * 2
	windowHeight     = unit.Dp(720)
	configWindowW    = configPanelWidth + windowPadding
	fullWindowW      = configPanelWidth + separatorWidth + logPanelWidth + windowPadding
)

// layout draws the entire UI as two columns.
func (a *appState) layout(gtx C) D {
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			// Left: config with fixed width
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Dp(configPanelWidth)
				gtx.Constraints.Max.X = gtx.Dp(configPanelWidth)
				return a.layoutConfigColumn(gtx)
			}),

			// Separator
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
					size := gtx.Constraints.Max
					size.X = gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
					return D{Size: size}
				})
			}),

			// Right: tabbed panel
			layout.Flexed(1, a.layoutRightPanel),
		)
	})
}

// layoutConfigColumn: top part scrolls, bottom (actions+progress+status) is pinned.
func (a *appState) layoutConfigColumn(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Top: header + paths + options — takes available space
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutHeader),
				layout.Rigid(spacer(10)),
				layout.Rigid(a.layoutPaths),
				layout.Rigid(spacer(10)),
				layout.Rigid(a.layoutOptions),
			)
		}),

		// Bottom: pinned — always at the same position
		layout.Rigid(spacer(10)),
		layout.Rigid(a.layoutActions),
		layout.Rigid(spacer(10)),
		layout.Rigid(a.layoutProgress),
		layout.Rigid(spacer(6)),
		layout.Rigid(a.layoutStatusBar),
	)
}

// --- Right panel with tabs ---

func (a *appState) layoutRightPanel(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Tab bar
		layout.Rigid(a.layoutTabBar),
		layout.Rigid(spacer(6)),

		// Tab content
		layout.Flexed(1, a.layoutTabContent),
	)
}

func (a *appState) layoutTabBar(gtx C) D {
	children := make([]layout.FlexChild, 0, len(a.tabs))
	for i, tab := range a.tabs {
		i, tab := i, tab
		children = append(children, layout.Rigid(func(gtx C) D {
			active := i == a.activeTab
			return a.layoutTab(gtx, i, tab.Title, active)
		}))
	}
	return layout.Flex{Alignment: layout.End}.Layout(gtx, children...)
}

func (a *appState) layoutTab(gtx C, index int, title string, active bool) D {
	return material.Clickable(gtx, &a.tabClicks[index], func(gtx C) D {
		return layout.Inset{
			Left: unit.Dp(16), Right: unit.Dp(16),
			Top: unit.Dp(8), Bottom: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			lbl := material.Body2(a.theme, title)
			lbl.Font.Weight = font.Bold
			if active {
				lbl.Color = colorPrimary
			} else {
				lbl.Color = colorTextMuted
			}
			dims := lbl.Layout(gtx)

			// Underline for active tab
			if active {
				lineHeight := gtx.Dp(unit.Dp(3))
				rect := image.Rectangle{
					Min: image.Point{X: 0, Y: dims.Size.Y},
					Max: image.Point{X: dims.Size.X, Y: dims.Size.Y + lineHeight},
				}
				paint.FillShape(gtx.Ops, colorPrimary, clip.Rect(rect).Op())
				dims.Size.Y += lineHeight
			}

			return dims
		})
	})
}

func (a *appState) layoutTabContent(gtx C) D {
	switch a.activeTab {
	case 0:
		return a.layoutLogContent(gtx)
	default:
		return D{}
	}
}

func (a *appState) layoutLogContent(gtx C) D {
	return bordered(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, colorLogBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

		if len(a.logEntries) == 0 {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
				lbl := material.Body2(a.theme, "Waiting for activity...")
				lbl.Color = colorTextMuted
				return lbl.Layout(gtx)
			})
		}

		return material.List(a.theme, &a.logList).Layout(gtx, len(a.logEntries), func(gtx C, i int) D {
			entry := a.logEntries[i]
			return layout.Inset{
				Left: unit.Dp(8), Right: unit.Dp(8),
				Top: unit.Dp(2), Bottom: unit.Dp(2),
			}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := material.Caption(a.theme, entry.Time.Format("15:04:05"))
						lbl.Color = colorTextMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(spacer(6)),
					layout.Rigid(func(gtx C) D {
						lbl := material.Caption(a.theme, fmt.Sprintf("[%-6s]", entry.Level))
						lbl.Font.Weight = font.Bold
						lbl.Color = levelColor(entry.Level)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(spacer(6)),
					layout.Flexed(1, func(gtx C) D {
						lbl := material.Caption(a.theme, entry.Message)
						lbl.Color = colorText
						return lbl.Layout(gtx)
					}),
				)
			})
		})
	})
}

// --- Header ---

func (a *appState) layoutHeader(gtx C) D {
	title := material.H5(a.theme, "SyncNorris")
	title.Color = colorPrimary
	title.Font.Weight = font.Bold
	return title.Layout(gtx)
}

// --- Paths ---

func (a *appState) layoutPaths(gtx C) D {
	return card(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lbl := material.Body1(a.theme, "Paths")
				lbl.Font.Weight = font.Bold
				lbl.Color = colorText
				return lbl.Layout(gtx)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				return a.layoutPathRow(gtx, "Source", &a.sourceEditor, &a.sourceBrowse,
					&a.sourceHistoryBtn, a.sourceHistoryOpen, a.settings.SourceHistory, a.sourceHistoryClicks[:])
			}),
			layout.Rigid(spacer(6)),
			layout.Rigid(func(gtx C) D {
				return a.layoutPathRow(gtx, "Destination", &a.destEditor, &a.destBrowse,
					&a.destHistoryBtn, a.destHistoryOpen, a.settings.DestHistory, a.destHistoryClicks[:])
			}),
		)
	})
}

func (a *appState) layoutPathRow(gtx C, label string, editor *widget.Editor, browseBtn *widget.Clickable,
	historyBtn *widget.Clickable, historyOpen bool, history []string, historyClicks []widget.Clickable) D {

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			lbl := material.Caption(a.theme, label)
			lbl.Font.Weight = font.Medium
			lbl.Color = colorTextMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(spacer(2)),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					if a.isRunning {
						gtx = gtx.Disabled()
					}
					return bordered(gtx, func(gtx C) D {
						return layout.UniformInset(unit.Dp(7)).Layout(gtx,
							material.Editor(a.theme, editor, "/path/to/directory").Layout,
						)
					})
				}),
				layout.Rigid(spacer(4)),
				layout.Rigid(func(gtx C) D {
					if a.isRunning || len(history) == 0 {
						gtx = gtx.Disabled()
					}
					btn := material.Button(a.theme, historyBtn, "▼")
					btn.Background = colorCancel
					btn.CornerRadius = unit.Dp(4)
					btn.Inset = layout.Inset{
						Top: unit.Dp(6), Bottom: unit.Dp(6),
						Left: unit.Dp(10), Right: unit.Dp(10),
					}
					return btn.Layout(gtx)
				}),
				layout.Rigid(spacer(4)),
				layout.Rigid(func(gtx C) D {
					if a.isRunning {
						gtx = gtx.Disabled()
					}
					btn := material.Button(a.theme, browseBtn, "Browse")
					btn.Background = colorPrimary
					btn.CornerRadius = unit.Dp(4)
					return btn.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D {
			if !historyOpen || len(history) == 0 {
				return D{}
			}
			return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				return bordered(gtx, func(gtx C) D {
					paint.FillShape(gtx.Ops, colorSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						a.historyItems(history, historyClicks)...,
					)
				})
			})
		}),
	)
}

func (a *appState) historyItems(history []string, clicks []widget.Clickable) []layout.FlexChild {
	n := len(history)
	if n > maxHistory {
		n = maxHistory
	}
	items := make([]layout.FlexChild, n)
	for i := 0; i < n; i++ {
		i := i
		path := history[i]
		items[i] = layout.Rigid(func(gtx C) D {
			return material.Clickable(gtx, &clicks[i], func(gtx C) D {
				return layout.Inset{
					Top: unit.Dp(4), Bottom: unit.Dp(4),
					Left: unit.Dp(8), Right: unit.Dp(8),
				}.Layout(gtx, func(gtx C) D {
					lbl := material.Body2(a.theme, path)
					lbl.Color = colorText
					return lbl.Layout(gtx)
				})
			})
		})
	}
	return items
}

// --- Options ---

func (a *appState) layoutOptions(gtx C) D {
	return card(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lbl := material.Body1(a.theme, "Options")
				lbl.Font.Weight = font.Bold
				lbl.Color = colorText
				return lbl.Layout(gtx)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				return a.radioRow(gtx, "Comparison", &a.compEnum, []radioOpt{
					{"hash", "Hash (SHA-256)"},
					{"md5", "MD5"},
					{"binary", "Binary"},
					{"namesize", "Name+Size"},
					{"timestamp", "Timestamp"},
				})
			}),
			layout.Rigid(spacer(6)),
			layout.Rigid(func(gtx C) D {
				return a.labeledInput(gtx, "Workers", &a.workersEditor, "5", 80)
			}),
			layout.Rigid(spacer(6)),
			layout.Rigid(func(gtx C) D {
				return a.labeledInput(gtx, "Excludes", &a.excludeEditor, "*.tmp, .git/", 0)
			}),
			layout.Rigid(spacer(6)),
			layout.Rigid(func(gtx C) D {
				if a.isRunning {
					gtx = gtx.Disabled()
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(material.CheckBox(a.theme, &a.dryRunCheck, "Dry Run").Layout),
					layout.Rigid(spacer(16)),
					layout.Rigid(material.CheckBox(a.theme, &a.deleteCheck, "Delete orphans").Layout),
					layout.Rigid(spacer(16)),
					layout.Rigid(material.CheckBox(a.theme, &a.createDestCheck, "Create dest.").Layout),
				)
			}),
		)
	})
}

type radioOpt struct {
	Key   string
	Label string
}

func (a *appState) radioRow(gtx C, label string, enum *widget.Enum, options []radioOpt) D {
	children := make([]layout.FlexChild, 0, len(options)+1)
	children = append(children, layout.Rigid(func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(100))
		lbl := material.Body2(a.theme, label)
		lbl.Font.Weight = font.Medium
		lbl.Color = colorTextMuted
		return lbl.Layout(gtx)
	}))
	for _, opt := range options {
		opt := opt
		children = append(children, layout.Rigid(func(gtx C) D {
			if a.isRunning {
				gtx = gtx.Disabled()
			}
			return material.RadioButton(a.theme, enum, opt.Key, opt.Label).Layout(gtx)
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (a *appState) labeledInput(gtx C, label string, editor *widget.Editor, hint string, maxWidth int) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(100))
			lbl := material.Body2(a.theme, label)
			lbl.Font.Weight = font.Medium
			lbl.Color = colorTextMuted
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D {
			if maxWidth > 0 {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(maxWidth))
			}
			if a.isRunning {
				gtx = gtx.Disabled()
			}
			return bordered(gtx, func(gtx C) D {
				return layout.UniformInset(unit.Dp(6)).Layout(gtx,
					material.Editor(a.theme, editor, hint).Layout,
				)
			})
		}),
	)
}

// --- Action buttons ---

func (a *appState) layoutActions(gtx C) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if a.isRunning {
				gtx = gtx.Disabled()
			}
			btn := material.Button(a.theme, &a.syncBtn, "Sync")
			btn.Background = colorSuccess
			btn.CornerRadius = unit.Dp(6)
			btn.Inset = layout.Inset{
				Top: unit.Dp(10), Bottom: unit.Dp(10),
				Left: unit.Dp(28), Right: unit.Dp(28),
			}
			btn.Font.Weight = font.Bold
			return btn.Layout(gtx)
		}),
		layout.Rigid(spacer(10)),
		layout.Rigid(func(gtx C) D {
			if a.isRunning {
				gtx = gtx.Disabled()
			}
			btn := material.Button(a.theme, &a.compareBtn, "Compare")
			btn.Background = colorInfo
			btn.CornerRadius = unit.Dp(6)
			btn.Inset = layout.Inset{
				Top: unit.Dp(10), Bottom: unit.Dp(10),
				Left: unit.Dp(28), Right: unit.Dp(28),
			}
			btn.Font.Weight = font.Bold
			return btn.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D {
			return D{Size: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Min.Y}}
		}),
		layout.Rigid(func(gtx C) D {
			if !a.isRunning {
				gtx = gtx.Disabled()
			}
			btn := material.Button(a.theme, &a.cancelBtn, "Cancel")
			btn.Background = colorCancel
			btn.CornerRadius = unit.Dp(6)
			btn.Inset = layout.Inset{
				Top: unit.Dp(10), Bottom: unit.Dp(10),
				Left: unit.Dp(20), Right: unit.Dp(20),
			}
			return btn.Layout(gtx)
		}),
	)
}

// --- Progress ---

func (a *appState) layoutProgress(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			bar := material.ProgressBar(a.theme, a.progress.Fraction)
			bar.Color = colorPrimary
			bar.TrackColor = colorBorder
			return bar.Layout(gtx)
		}),
		layout.Rigid(spacer(4)),
		layout.Rigid(func(gtx C) D {
			info := fmt.Sprintf("%.0f%%", a.progress.Fraction*100)
			if a.progress.TotalFiles > 0 {
				info += fmt.Sprintf("  |  %d / %d files", a.progress.CurrentFile, a.progress.TotalFiles)
			}
			if a.isRunning {
				dots := int(gtx.Now.UnixMilli()/500) % 4 // cycle 0..3 every 500ms
				info += fmt.Sprintf("  |  Processing%s", "...."[:dots+1])
				// Request continuous redraw for animation
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(500 * time.Millisecond)})
			} else if a.progress.Fraction >= 1 {
				info += "  |  Done"
			}
			lbl := material.Caption(a.theme, info)
			lbl.Color = colorTextMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(spacer(2)),
		layout.Rigid(func(gtx C) D {
			s := a.progress.Stats
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return a.statBadge(gtx, fmt.Sprintf("Copied: %d", s.Copied), colorSuccess)
				}),
				layout.Rigid(spacer(10)),
				layout.Rigid(func(gtx C) D {
					return a.statBadge(gtx, fmt.Sprintf("Updated: %d", s.Updated), colorInfo)
				}),
				layout.Rigid(spacer(10)),
				layout.Rigid(func(gtx C) D {
					return a.statBadge(gtx, fmt.Sprintf("Identical: %d", s.Skipped), colorSkip)
				}),
				layout.Rigid(spacer(10)),
				layout.Rigid(func(gtx C) D {
					if s.Errors == 0 {
						return D{}
					}
					return a.statBadge(gtx, fmt.Sprintf("Errors: %d", s.Errors), colorError)
				}),
			)
		}),
		// Bandwidth graph
		layout.Rigid(spacer(4)),
		layout.Rigid(a.layoutBandwidth),
	)
}

const graphHeight = unit.Dp(50)

func (a *appState) layoutBandwidth(gtx C) D {
	current := a.bandwidth.Current()
	avg := a.bandwidth.Average()
	samples := a.bandwidth.Samples()

	// Text labels
	bwText := fmt.Sprintf("Current: %s/s  |  Average: %s/s", formatSize(int64(current)), formatSize(int64(avg)))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			lbl := material.Caption(a.theme, bwText)
			lbl.Color = colorTextMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(spacer(2)),
		layout.Rigid(func(gtx C) D {
			// Graph area
			h := gtx.Dp(graphHeight)
			w := gtx.Constraints.Max.X
			size := image.Point{X: w, Y: h}
			gtx.Constraints = layout.Exact(size)

			// Background
			paint.FillShape(gtx.Ops, colorLogBg, clip.Rect{Max: size}.Op())

			if len(samples) < 2 {
				// Not enough data — draw empty box
				return bordered(gtx, func(gtx C) D {
					paint.FillShape(gtx.Ops, colorLogBg, clip.Rect{Max: size}.Op())
					return D{Size: size}
				})
			}

			// Find max value for Y scale (include average so line is always visible)
			maxVal := avg
			for _, s := range samples {
				if s.BytesPerSec > maxVal {
					maxVal = s.BytesPerSec
				}
			}
			if maxVal == 0 {
				maxVal = 1 // avoid division by zero
			}

			// Draw filled area chart
			n := len(samples)
			stepX := float32(w) / float32(n-1)

			var path clip.Path
			path.Begin(gtx.Ops)
			// Start at bottom-left
			path.MoveTo(f32Point(0, float32(h)))
			// Line to first data point
			y0 := float32(h) - float32(h)*float32(samples[0].BytesPerSec/maxVal)
			path.LineTo(f32Point(0, y0))
			// Data points
			for i := 1; i < n; i++ {
				x := stepX * float32(i)
				y := float32(h) - float32(h)*float32(samples[i].BytesPerSec/maxVal)
				path.LineTo(f32Point(x, y))
			}
			// Close to bottom-right then bottom-left
			path.LineTo(f32Point(float32(w), float32(h)))
			path.Close()

			// Fill with semi-transparent primary color
			fillColor := colorPrimary
			fillColor.A = 0x40
			paint.FillShape(gtx.Ops, fillColor, clip.Outline{Path: path.End()}.Op())

			// Draw the line on top
			var linePath clip.Path
			linePath.Begin(gtx.Ops)
			ly0 := float32(h) - float32(h)*float32(samples[0].BytesPerSec/maxVal)
			linePath.MoveTo(f32Point(0, ly0))
			for i := 1; i < n; i++ {
				x := stepX * float32(i)
				y := float32(h) - float32(h)*float32(samples[i].BytesPerSec/maxVal)
				linePath.LineTo(f32Point(x, y))
			}
			lineColor := colorPrimary
			lineColor.A = 0xa0
			paint.FillShape(gtx.Ops, lineColor,
				clip.Stroke{Path: linePath.End(), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())

			// Average line (dashed red) — simulated with short segments
			if avg > 0 {
				avgY := float32(h) - float32(h)*float32(avg/maxVal)
				dashLen := float32(gtx.Dp(unit.Dp(6)))
				gapLen := float32(gtx.Dp(unit.Dp(4)))
				x := float32(0)
				for x < float32(w) {
					endX := x + dashLen
					if endX > float32(w) {
						endX = float32(w)
					}
					var dash clip.Path
					dash.Begin(gtx.Ops)
					dash.MoveTo(f32Point(x, avgY))
					dash.LineTo(f32Point(endX, avgY))
					paint.FillShape(gtx.Ops, colorError,
						clip.Stroke{Path: dash.End(), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
					x = endX + gapLen
				}
			}

			// Border around the graph
			paint.FillShape(gtx.Ops, colorBorder,
				clip.Stroke{
					Path:  clip.Rect{Max: size}.Path(),
					Width: float32(gtx.Dp(unit.Dp(1))),
				}.Op())

			return D{Size: size}
		}),
	)
}

func (a *appState) statBadge(gtx C, text string, clr color.NRGBA) D {
	lbl := material.Caption(a.theme, text)
	lbl.Font.Weight = font.Bold
	lbl.Color = clr
	return lbl.Layout(gtx)
}

// --- Status bar ---

func (a *appState) layoutStatusBar(gtx C) D {
	lbl := material.Caption(a.theme, a.statusMsg)
	lbl.Font.Weight = font.Medium
	switch a.statusLevel {
	case "error":
		lbl.Color = colorError
	case "success":
		lbl.Color = colorSuccess
	default:
		lbl.Color = colorTextMuted
	}
	return lbl.Layout(gtx)
}

// --- Helpers ---

func spacer(dp int) func(C) D {
	return func(gtx C) D {
		return layout.Spacer{Height: unit.Dp(dp), Width: unit.Dp(dp)}.Layout(gtx)
	}
}

// card draws content inside a rounded surface with a subtle background.
func card(gtx C, content func(C) D) D {
	return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		return widget.Border{
			Color:        colorBorder,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			r := clip.RRect{
				Rect: image.Rectangle{Max: gtx.Constraints.Max},
				SE:   gtx.Dp(unit.Dp(8)), SW: gtx.Dp(unit.Dp(8)),
				NE:   gtx.Dp(unit.Dp(8)), NW: gtx.Dp(unit.Dp(8)),
			}
			paint.FillShape(gtx.Ops, colorSurface, r.Op(gtx.Ops))
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
		})
	})
}

func bordered(gtx C, w func(C) D) D {
	return widget.Border{
		Color:        colorBorder,
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return w(gtx)
	})
}

func levelColor(level string) color.NRGBA {
	switch level {
	case "COPY", "DONE":
		return colorSuccess
	case "UPDATE":
		return colorInfo
	case "SKIP":
		return colorSkip
	case "DELETE":
		return colorWarning
	case "ERROR":
		return colorError
	case "SCAN", "INFO":
		return colorTextMuted
	default:
		return colorText
	}
}

func f32Point(x, y float32) f32.Point {
	return f32.Point{X: x, Y: y}
}
