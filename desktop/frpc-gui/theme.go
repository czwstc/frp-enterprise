package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

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
	"gear":  `<circle cx="8" cy="8" r="2.2"/><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4"/>`,
	"globe": `<circle cx="8" cy="8" r="6"/><path d="M2 8h12M8 2c2 2 2 10 0 12M8 2c-2 2-2 10 0 12"/>`,
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

//go:embed brand_logo.png
var huidingLogoPNG []byte

// brandLogo is the logo shown in the sidebar: the server's, or the Huiding
// logo, prepared once per change with the backdrop the server chose.
type brandLogo struct {
	key      string
	img      ui.ImageSource
	plate    bool
	fallback bool
}

func (b *brandLogo) update(dataURL, backdrop string) {
	if backdrop == "" {
		backdrop = "halo"
	}
	key := backdrop + "|" + dataURL
	if key == b.key && b.img != nil {
		return
	}
	b.key, b.img, b.plate, b.fallback = key, nil, backdrop == "plate", false
	raw, kind := decodeDataURL(dataURL)
	if raw == nil {
		raw, kind, b.fallback = huidingLogoPNG, "png", true
	}
	if kind == "svg+xml" {
		svg, err := ui.ParseSVG(raw)
		if err == nil {
			b.img = svg
			// An SVG cannot be traced for a halo: give it a plate instead.
			b.plate = backdrop != "none"
			return
		}
		raw, b.fallback = huidingLogoPNG, true
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		src, _, _ = image.Decode(bytes.NewReader(huidingLogoPNG))
		b.fallback = true
	}
	// Scale to the size the sidebar shows (twice its DIPs, for Retina), so
	// the halo is a few screen pixels wide rather than shrinking away.
	src = scaleToHeight(src, 2*logoHeight)
	if backdrop == "halo" {
		src = withHalo(src)
	}
	b.img = ui.NewBitmap(src)
}

func decodeDataURL(dataURL string) ([]byte, string) {
	const prefix = "data:image/"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, ""
	}
	kind, payload, ok := strings.Cut(dataURL[len(prefix):], ";base64,")
	if !ok {
		return nil, ""
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, ""
	}
	return raw, kind
}

// logoHeight is the logo's height in the sidebar, in DIPs.
const logoHeight = 30

func scaleToHeight(src image.Image, h int) image.Image {
	b := src.Bounds()
	if b.Dy() <= h {
		return src
	}
	w := b.Dx() * h / b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// withHalo draws a soft black glow traced around the logo's shape under it,
// so a white logo on a transparent background reads on a light sidebar.
func withHalo(src image.Image) image.Image {
	const pad, grow, blur = 5, 1, 2
	b := src.Bounds()
	w, h := b.Dx()+2*pad, b.Dy()+2*pad
	alpha := make([]float32, w*h)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			alpha[(y+pad)*w+x+pad] = float32(a) / 0xffff
		}
	}
	// Grow the shape by a few pixels, then soften it with a box blur.
	grown := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m := float32(0)
			for dy := -grow; dy <= grow; dy++ {
				for dx := -grow; dx <= grow; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < w && yy < h && alpha[yy*w+xx] > m {
						m = alpha[yy*w+xx]
					}
				}
			}
			grown[y*w+x] = m
		}
	}
	soft := boxBlur(grown, w, h, blur)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			halo := min(1, soft[y*w+x]*1.6) * 0.9
			out.SetNRGBA(x, y, color.NRGBA{A: uint8(halo * 255)})
		}
	}
	draw.Draw(out, image.Rect(pad, pad, pad+b.Dx(), pad+b.Dy()), src, b.Min, draw.Over)
	return out
}

func boxBlur(in []float32, w, h, r int) []float32 {
	tmp := make([]float32, len(in))
	out := make([]float32, len(in))
	n := float32(2*r + 1)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum := float32(0)
			for d := -r; d <= r; d++ {
				if xx := x + d; xx >= 0 && xx < w {
					sum += in[y*w+xx]
				}
			}
			tmp[y*w+x] = sum / n
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum := float32(0)
			for d := -r; d <= r; d++ {
				if yy := y + d; yy >= 0 && yy < h {
					sum += tmp[yy*w+x]
				}
			}
			out[y*w+x] = sum / n
		}
	}
	return out
}

// brandMark shows the logo alone, on a dark plate when the server asks.
func (a *app) brandMark(c *ui.Context, _ palette) {
	box := ui.Row(c).AlignItems(ui.Center)
	if a.logo.plate {
		box.Background(ui.Hex("#1b2233")).Radius(8).Padding(6, 10)
	}
	box.Children(func() {
		if a.logo.img != nil {
			ui.Image(c, a.logo.img).Height(logoHeight + 5).MaxWidth(190).Fit(ui.Contain).Label(siteName(a.status.Branding))
		}
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
