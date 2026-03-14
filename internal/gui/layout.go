package gui

import (
	"fmt"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
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

// logPanelWidth is the fixed width of the log side panel.
const logPanelWidth = unit.Dp(420)

// layout draws the entire UI as two columns:
//   Left:  config panel (paths, options, actions, progress, status)
//   Right: log panel (togglable)
func (a *appState) layout(gtx C) D {
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
		if !a.logVisible {
			// Single column — config only
			return a.layoutConfigColumn(gtx)
		}

		// Two columns: config (flex) | separator | log (rigid)
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			// Left: config takes remaining space
			layout.Flexed(1, a.layoutConfigColumn),

			// Separator
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
					size := gtx.Constraints.Max
					size.X = gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
					return D{Size: size}
				})
			}),

			// Right: log panel with fixed width
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Dp(logPanelWidth)
				gtx.Constraints.Max.X = gtx.Dp(logPanelWidth)
				return a.layoutLogPanel(gtx)
			}),
		)
	})
}

// layoutConfigColumn renders the left configuration column.
func (a *appState) layoutConfigColumn(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutHeader),
		layout.Rigid(spacer(12)),
		layout.Rigid(a.layoutPaths),
		layout.Rigid(spacer(12)),
		layout.Rigid(a.layoutOptions),
		layout.Rigid(spacer(12)),
		layout.Rigid(a.layoutActions),
		layout.Rigid(spacer(12)),
		layout.Rigid(a.layoutProgress),

		// Push status bar to bottom
		layout.Flexed(1, func(gtx C) D { return D{} }),

		layout.Rigid(a.layoutStatusBar),
	)
}

// layoutLogPanel renders the right log panel (full height).
func (a *appState) layoutLogPanel(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header with hide button
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := material.Body1(a.theme, "Activity Log")
					lbl.Font.Weight = font.Bold
					lbl.Color = colorText
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx C) D { return D{} }),
				layout.Rigid(func(gtx C) D {
					btn := material.Button(a.theme, &a.toggleLog, "Hide")
					btn.Background = colorCancel
					btn.CornerRadius = unit.Dp(4)
					btn.Inset = layout.Inset{
						Top: unit.Dp(4), Bottom: unit.Dp(4),
						Left: unit.Dp(12), Right: unit.Dp(12),
					}
					return btn.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(spacer(6)),

		// Log list fills remaining vertical space
		layout.Flexed(1, func(gtx C) D {
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
		}),
	)
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
	return section(gtx, a.theme, "Paths", func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return a.layoutPathRow(gtx, "Source", &a.sourceEditor, &a.sourceBrowse)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				return a.layoutPathRow(gtx, "Destination", &a.destEditor, &a.destBrowse)
			}),
		)
	})
}

func (a *appState) layoutPathRow(gtx C, label string, editor *widget.Editor, browseBtn *widget.Clickable) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			lbl := material.Body2(a.theme, label)
			lbl.Font.Weight = font.Medium
			lbl.Color = colorTextMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(spacer(4)),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					if a.isRunning {
						gtx = gtx.Disabled()
					}
					return bordered(gtx, func(gtx C) D {
						return layout.UniformInset(unit.Dp(8)).Layout(gtx,
							material.Editor(a.theme, editor, "/path/to/directory").Layout,
						)
					})
				}),
				layout.Rigid(spacer(8)),
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
	)
}

// --- Options ---

func (a *appState) layoutOptions(gtx C) D {
	return section(gtx, a.theme, "Options", func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return a.radioRow(gtx, "Mode", &a.modeEnum, []radioOpt{
					{"oneway", "One-Way"},
					{"bidirectional", "Bidirectional"},
				})
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
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				return a.labeledInput(gtx, "Workers", &a.workersEditor, "5", 80)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				return a.labeledInput(gtx, "Exclude patterns", &a.excludeEditor, "*.tmp, .git/", 0)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx C) D {
				if a.isRunning {
					gtx = gtx.Disabled()
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(material.CheckBox(a.theme, &a.dryRunCheck, "Dry Run").Layout),
					layout.Rigid(spacer(16)),
					layout.Rigid(material.CheckBox(a.theme, &a.deleteCheck, "Delete orphans").Layout),
					layout.Rigid(spacer(16)),
					layout.Rigid(material.CheckBox(a.theme, &a.createDestCheck, "Create destination").Layout),
				)
			}),
			// Bidirectional-only options
			layout.Rigid(func(gtx C) D {
				if a.modeEnum.Value != "bidirectional" {
					return D{}
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(spacer(8)),
					layout.Rigid(func(gtx C) D {
						return a.radioRow(gtx, "Conflict resolution", &a.conflictEnum, []radioOpt{
							{"newer", "Newer wins"},
							{"source-wins", "Source wins"},
							{"dest-wins", "Dest wins"},
							{"both", "Keep both"},
						})
					}),
					layout.Rigid(spacer(4)),
					layout.Rigid(func(gtx C) D {
						if a.isRunning {
							gtx = gtx.Disabled()
						}
						return material.CheckBox(a.theme, &a.statefulCheck, "Stateful (track changes between syncs)").Layout(gtx)
					}),
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
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(120))
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
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(120))
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
			btn.CornerRadius = unit.Dp(4)
			btn.Inset = layout.Inset{
				Top: unit.Dp(10), Bottom: unit.Dp(10),
				Left: unit.Dp(24), Right: unit.Dp(24),
			}
			btn.Font.Weight = font.Bold
			return btn.Layout(gtx)
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx C) D {
			if a.isRunning {
				gtx = gtx.Disabled()
			}
			btn := material.Button(a.theme, &a.compareBtn, "Compare")
			btn.Background = colorInfo
			btn.CornerRadius = unit.Dp(4)
			btn.Inset = layout.Inset{
				Top: unit.Dp(10), Bottom: unit.Dp(10),
				Left: unit.Dp(24), Right: unit.Dp(24),
			}
			btn.Font.Weight = font.Bold
			return btn.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx C) D { return D{} }),
		// Show/Hide Log toggle
		layout.Rigid(func(gtx C) D {
			label := "Show Log"
			if a.logVisible {
				label = "Hide Log"
			}
			btn := material.Button(a.theme, &a.toggleLog, label)
			btn.Background = colorCancel
			btn.CornerRadius = unit.Dp(4)
			return btn.Layout(gtx)
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx C) D {
			if !a.isRunning {
				gtx = gtx.Disabled()
			}
			btn := material.Button(a.theme, &a.cancelBtn, "Cancel")
			btn.Background = colorCancel
			btn.CornerRadius = unit.Dp(4)
			return btn.Layout(gtx)
		}),
	)
}

// --- Progress ---

func (a *appState) layoutProgress(gtx C) D {
	if !a.isRunning && a.progress.Fraction == 0 {
		return D{}
	}
	return section(gtx, a.theme, "Progress", func(gtx C) D {
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
				if a.progress.CurrentPath != "" {
					path := a.progress.CurrentPath
					if len(path) > 50 {
						path = "..." + path[len(path)-47:]
					}
					info += fmt.Sprintf("  |  %s", path)
				}
				lbl := material.Body2(a.theme, info)
				lbl.Color = colorTextMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(spacer(4)),
			layout.Rigid(func(gtx C) D {
				s := a.progress.Stats
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return a.statBadge(gtx, fmt.Sprintf("Copied: %d", s.Copied), colorSuccess)
					}),
					layout.Rigid(spacer(12)),
					layout.Rigid(func(gtx C) D {
						return a.statBadge(gtx, fmt.Sprintf("Updated: %d", s.Updated), colorInfo)
					}),
					layout.Rigid(spacer(12)),
					layout.Rigid(func(gtx C) D {
						return a.statBadge(gtx, fmt.Sprintf("Identical: %d", s.Skipped), colorSkip)
					}),
					layout.Rigid(spacer(12)),
					layout.Rigid(func(gtx C) D {
						if s.Errors == 0 {
							return D{}
						}
						return a.statBadge(gtx, fmt.Sprintf("Errors: %d", s.Errors), colorError)
					}),
				)
			}),
		)
	})
}

func (a *appState) statBadge(gtx C, text string, clr color.NRGBA) D {
	lbl := material.Body2(a.theme, text)
	lbl.Font.Weight = font.Medium
	lbl.Color = clr
	return lbl.Layout(gtx)
}

// --- Status bar ---

func (a *appState) layoutStatusBar(gtx C) D {
	lbl := material.Body2(a.theme, a.statusMsg)
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

func section(gtx C, th *material.Theme, title string, content func(C) D) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			lbl := material.Body1(th, title)
			lbl.Font.Weight = font.Bold
			lbl.Color = colorText
			return lbl.Layout(gtx)
		}),
		layout.Rigid(spacer(6)),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, content)
		}),
	)
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
