package api

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Go's bundled fonts are redistributable and keep image generation offline,
// deterministic and CGO-free. Faces are request-local (they are not concurrent).
var cardRegular, _ = opentype.Parse(goregular.TTF)
var cardBold, _ = opentype.Parse(gobold.TTF)

func renderShareCard(site shareSite, p sharePreview) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 630))
	bg := color.RGBA{13, 17, 23, 255}
	panel := color.RGBA{22, 29, 37, 255}
	line := color.RGBA{48, 59, 71, 255}
	white := color.RGBA{240, 246, 252, 255}
	muted := color.RGBA{157, 173, 188, 255}
	teal := color.RGBA{46, 218, 194, 255}
	gold := color.RGBA{233, 184, 85, 255}
	rect := func(x, y, w, h int, c color.RGBA) {
		draw.Draw(img, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Src)
	}
	rect(0, 0, 1200, 630, bg)
	rect(0, 0, 900, 6, teal)
	rect(900, 0, 300, 6, gold)
	faces := map[string]font.Face{}
	face := func(size int, bold bool) font.Face {
		key := string(rune(size))
		if bold {
			key += "b"
		}
		if f, ok := faces[key]; ok {
			return f
		}
		fnt := cardRegular
		if bold {
			fnt = cardBold
		}
		f, _ := opentype.NewFace(fnt, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
		faces[key] = f
		return f
	}
	defer func() {
		for _, f := range faces {
			f.Close()
		}
	}()
	text := func(s string, x, y, size, maxWidth int, bold bool, c color.RGBA) {
		f := face(size, bold)
		// Keep unsupported glyphs visible instead of silently dropping node names.
		s = strings.Map(func(r rune) rune {
			if _, ok := f.GlyphAdvance(r); !ok {
				return '□'
			}
			return r
		}, s)
		if font.MeasureString(f, s).Ceil() > maxWidth {
			r := []rune(s)
			for len(r) > 0 && font.MeasureString(f, string(r)+"…").Ceil() > maxWidth {
				r = r[:len(r)-1]
			}
			s = string(r) + "…"
		}
		d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	// Small brand mark; the node identity carries the visual emphasis.
	poly := func(points [][2]float32, c color.RGBA) {
		z := vector.NewRasterizer(1200, 630)
		z.MoveTo(points[0][0], points[0][1])
		for _, p := range points[1:] {
			z.LineTo(p[0], p[1])
		}
		z.ClosePath()
		z.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
	}
	poly([][2]float32{{56, 91}, {76, 64}, {91, 77}, {114, 48}, {141, 91}, {131, 96}, {113, 67}, {93, 94}, {78, 80}, {65, 98}}, teal)
	poly([][2]float32{{64, 108}, {81, 88}, {95, 100}, {114, 79}, {137, 109}, {130, 115}, {113, 94}, {96, 115}, {82, 103}, {70, 115}}, gold)
	text(site.Name, 162, 91, 32, 680, true, white)
	text("MESHCORE OBSERVATORY", 56, 161, 17, 800, true, muted)
	label := "PUBLIC PAGE"
	if p.Node != nil {
		label = strings.ToUpper(shareRole(p.Node.Role))
	}
	text(label, 922, 87, 17, 230, true, teal)
	// Two title lines, wrapping at a word boundary when one is available.
	title := cleanShareText(p.Title, 100)
	runes := []rune(title)
	cut := len(runes)
	for cut > 0 && font.MeasureString(face(54, true), string(runes[:cut])).Ceil() > 810 {
		cut--
	}
	if cut < len(runes) {
		lineEnd := cut
		for i := cut; i > cut/2; i-- {
			if runes[i-1] == ' ' {
				lineEnd = i - 1
				break
			}
		}
		text(string(runes[:lineEnd]), 56, 237, 54, 810, true, white)
		text(strings.TrimSpace(string(runes[lineEnd:])), 56, 304, 54, 810, true, white)
	} else {
		text(title, 56, 237, 54, 810, true, white)
	}
	// Identicon encodes the public key (or route); decorative, not telemetry.
	identity := p.Path
	if p.Node != nil {
		identity = p.Node.PublicKey
	}
	sum := sha256.Sum256([]byte(identity))
	rect(932, 172, 212, 212, panel)
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			if sum[y*3+x]&1 != 0 {
				rect(953+x*34, 193+y*34, 28, 28, teal)
				rect(953+(4-x)*34, 193+y*34, 28, 28, teal)
			}
		}
	}
	if p.Node != nil {
		key := p.Node.PublicKey
		text(key[:16]+"…"+key[len(key)-12:], 56, 352, 22, 820, false, muted)
	} else {
		text(p.Description, 56, 352, 22, 820, false, muted)
	}
	rect(56, 411, 1088, 1, line)
	if p.Node != nil {
		text("LAST ADVERT OBSERVED", 56, 454, 16, 600, true, muted)
		text(shareDate(p.Node.LastAdvert), 56, 497, 29, 610, true, white)
		rect(717, 442, 1, 63, line)
		text("FIRST OBSERVED", 760, 454, 16, 380, true, muted)
		text(shareDate(p.Node.FirstSeen), 760, 497, 22, 380, false, white)
	} else {
		text("EXPLORE THE NETWORK", 56, 454, 16, 900, true, muted)
		text("Nodes, coverage and the connections between them.", 56, 497, 28, 1080, false, white)
	}
	text("PUBLIC RECORD  /  OPEN FOR CURRENT DETAILS", 56, 583, 15, 680, true, muted)
	host := strings.TrimPrefix(strings.TrimPrefix(site.URL, "https://"), "http://")
	if host == "" {
		host = "MeshCore · Ridgeline"
	}
	text(host, 760, 583, 18, 384, false, muted)
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}
