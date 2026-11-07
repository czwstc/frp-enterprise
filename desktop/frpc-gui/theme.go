package main

import (
	"bytes"
	"encoding/base64"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
)

// AppName is the product name shown in the window and the login page.
const AppName = "HDIT-FRPE-Client"

const defaultAccent = "#1f5eff"

// palette is the Pulse look: a gray source list beside a light canvas of
// white cards, and one color per kind of metric.
type palette struct {
	canvas, side, sideHover, card, line, dotOff               ui.Color
	blue, green, pink, orange, teal, violet, amber, red, gray ui.Color
	accentSoft                                                ui.Color
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			canvas: ui.Hex("#111317"), side: ui.Hex("#1b1e24"), sideHover: ui.Hex("#262a32"),
			card: ui.Hex("#1a1d23"), line: ui.Hex("#2a2e37"), dotOff: ui.Hex("#262a33"),
			blue: ui.Hex("#5b8cff"), green: ui.Hex("#3cc47a"), pink: ui.Hex("#f06b98"), orange: ui.Hex("#f59a4a"),
			teal: ui.Hex("#2cc5c9"), violet: ui.Hex("#9b85ff"), amber: ui.Hex("#f0b93a"), red: ui.Hex("#f26b65"),
			gray: ui.Hex("#6b7280"), accentSoft: mix(t.Accent, ui.Hex("#1a1d23"), 0.78),
		}
	}
	return palette{
		canvas: ui.Hex("#f6f7f9"), side: ui.Hex("#eef0f3"), sideHover: ui.Hex("#e2e5ea"),
		card: ui.Hex("#ffffff"), line: ui.Hex("#e4e7ec"), dotOff: ui.Hex("#e7eaf0"),
		blue: ui.Hex("#2f6bff"), green: ui.Hex("#22a35a"), pink: ui.Hex("#e5487c"), orange: ui.Hex("#ef7d22"),
		teal: ui.Hex("#11a3a8"), violet: ui.Hex("#7b5cf0"), amber: ui.Hex("#d99a0b"), red: ui.Hex("#e0443e"),
		gray: ui.Hex("#8a919e"), accentSoft: mix(t.Accent, ui.Hex("#ffffff"), 0.88),
	}
}

// mix blends a toward b by f (0..1).
func mix(a, b ui.Color, f float32) ui.Color {
	m := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*f + 0.5) }
	return ui.Color{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 255}
}

// applyTheme sets the server's accent on the theme that follows the system.
func applyTheme(c *ui.Context, accent string) palette {
	t := *c.Theme()
	if !validHex(accent) {
		accent = defaultAccent
	}
	a := ui.Hex(accent)
	if t.Dark {
		a = mix(a, ui.Hex("#ffffff"), 0.18)
	}
	t.Accent = a
	t.AccentHover = mix(a, ui.Hex("#000000"), 0.08)
	t.AccentPressed = mix(a, ui.Hex("#000000"), 0.16)
	t.AccentText = ui.Hex("#ffffff")
	t.Focus = a
	t.Radius = 8
	p := paletteFor(&t)
	t.Background = p.canvas
	c.SetTheme(&t)
	return p
}

func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// Sidebar icons: strokes in white on a colored square.
var iconPaths = map[string]string{
	"home":  `<path d="M2 8l6-5 6 5v6H2z"/>`,
	"link":  `<path d="M6.5 9.5l3-3M5 11a2.5 2.5 0 010-3.5L7 5.5M11 5a2.5 2.5 0 010 3.5L9 10.5"/>`,
	"grid":  `<path d="M2 2h5v5H2zM9 2h5v5H9zM2 9h5v5H2zM9 9h5v5H9z"/>`,
	"bell":  `<path d="M4 11V7a4 4 0 018 0v4l1 1H3zM7 14h2"/>`,
	"doc":   `<path d="M4 1.5h6l3 3v10H4zM10 1.5v3h3"/>`,
	"lock":  `<path d="M4 7h8v7H4zM5.5 7V5a2.5 2.5 0 015 0v2"/>`,
	"check": `<path d="M3.5 8.5l3 3L12.5 5"/>`,
	"pause": `<path d="M5.5 3.5v9M10.5 3.5v9"/>`,
	"x":     `<path d="M4 4l8 8M12 4l-8 8"/>`,
}

var icons = func() map[string]*ui.SVG {
	out := map[string]*ui.SVG{}
	for name, p := range iconPaths {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + p + `</svg>`
		out[name] = ui.MustParseSVG([]byte(svg))
	}
	return out
}()

// iconSquare draws an icon in white on a rounded square of color bg.
func iconSquare(c *ui.Context, name string, bg ui.Color, size float32) {
	ui.Column(c).Size(size, size).Radius(size * 0.28).Background(bg).Center().TextColor(ui.RGB(255, 255, 255)).Children(func() {
		ui.Icon(c, icons[name]).Size(size*0.62, size*0.62)
	})
}

// brandLogo is the server's logo, decoded once per change.
type brandLogo struct {
	src string
	img ui.ImageSource
}

func (b *brandLogo) update(dataURL string) {
	if dataURL == b.src {
		return
	}
	b.src, b.img = dataURL, nil
	const prefix = "data:image/"
	if !strings.HasPrefix(dataURL, prefix) {
		return
	}
	kind, payload, ok := strings.Cut(dataURL[len(prefix):], ";base64,")
	if !ok {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return
	}
	if kind == "svg+xml" {
		if svg, err := ui.ParseSVG(raw); err == nil {
			b.img = svg
		}
		return
	}
	if bmp, err := ui.DecodeBitmap(bytes.Clone(raw)); err == nil {
		b.img = bmp
	}
}

// brandMark shows the server's logo, or the Huiding mark until one is set.
func (a *app) brandMark(c *ui.Context, _ palette) {
	t := c.Theme()
	if a.logo.img != nil {
		ui.Image(c, a.logo.img).Height(28).MaxWidth(150).Fit(ui.Contain)
		return
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "hJ").Font("Georgia").Italic().Bold().FontSize(22).TextColor(t.Accent)
		ui.Column(c).Children(func() {
			ui.Text(c, "汇鼎科技").Bold().FontSize(12)
			ui.Text(c, "HUIDING GROUP").FontSize(7).LetterSpacing(0.8).TextColor(t.TextMuted)
		})
	})
}

// siteName is the name the server's administrator chose.
func siteName(b enterprise.Branding) string {
	if b.SiteName != "" {
		return b.SiteName
	}
	return "HDIT FRPE"
}

// card is a white panel with a thin border, as Pulse's.
func card(c *ui.Context, p palette) ui.Element {
	return ui.Column(c).Background(p.card).Border(1, p.line).Radius(12).Padding(12, 14).Gap(8)
}

// cardHeader is a card's title row: icon, title and what follows on the right.
func cardHeader(c *ui.Context, icon string, color ui.Color, title string, right func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		iconSquare(c, icon, color, 20)
		ui.Text(c, title).Bold().FontSize(12.5).Grow(1)
		if right != nil {
			right()
		}
	})
}

// dotMatrix draws values (0..1) as columns of lit dots, as Pulse's charts.
func dotMatrix(c *ui.Context, values []float32, on, off ui.Color, height float32) ui.Element {
	return ui.Column(c).Height(height).FillWidth().Draw(func(pt *ui.Painter, r ui.Rect) {
		const step, dot = 4, 2.4
		cols := int(r.W / step)
		rows := int(r.H / step)
		if cols <= 0 || rows <= 0 {
			return
		}
		vals := values
		if len(vals) > cols {
			vals = vals[len(vals)-cols:]
		}
		offset := cols - len(vals)
		for x := 0; x < cols; x++ {
			v := float32(0)
			if x >= offset {
				v = vals[x-offset]
			}
			lit := 0
			if v > 0 {
				lit = max(1, int(v*float32(rows)+0.5))
			}
			for y := 0; y < rows; y++ {
				level := rows - y
				col := off
				if level <= lit {
					col = on
					col.A = uint8(255 * (0.45 + 0.55*float32(level)/float32(rows)))
				}
				pt.Fill(ui.Rect{X: r.X + float32(x)*step, Y: r.Y + float32(y)*step, W: dot, H: dot}, col, 0.6)
			}
		}
	})
}

// tag is a small colored pill.
func tag(c *ui.Context, text string, color ui.Color) {
	bg := color
	bg.A = 40
	ui.Text(c, text).FontSize(10.5).Bold().TextColor(color).Background(bg).Padding(1, 7).Radius(10)
}
