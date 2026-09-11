package api

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/vector"
)

// Bundled fonts keep generation offline, deterministic and CGO-free.
// Each render owns its font faces; opentype faces are not concurrent.
var cardRegular, _ = opentype.Parse(goregular.TTF)
var cardBold, _ = opentype.Parse(gobold.TTF)

// Keep the redistribution notice with the font in the embedded file system.
//
//go:embed fonts/JetBrainsMono-Regular.ttf fonts/OFL.txt
var cardFontFiles embed.FS

var cardMono = func() *opentype.Font {
	b, err := cardFontFiles.ReadFile("fonts/JetBrainsMono-Regular.ttf")
	if err != nil {
		panic(err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		panic(err)
	}
	return f
}()

type shareCardLayout struct {
	Width, Height, Margin                   int
	BrandY, BrandSize                       int
	IconX, IconY, IconSize                  int
	RoleX, RoleY, RoleSize                  int
	TitleY, TitleSize, TitleWidth, TitleGap int
	RuleY, KeyLabelY, KeyY                  int
	KeySize, KeyGap, KeyDigits              int
}

// Fixed output sizes bound rendering cost. Taller cards reflow their identity
// and key, rather than cropping or stretching the landscape card. Coordinates
// are baselines; story content stays clear of the top/bottom overlay areas.
var shareCardLayouts = map[string]shareCardLayout{
	"wide": {
		Width: 1200, Height: 630, Margin: 56, BrandY: 109, BrandSize: 45,
		IconX: 932, IconY: 182, IconSize: 212, RoleX: 56, RoleY: 193, RoleSize: 45,
		TitleY: 285, TitleSize: 80, TitleWidth: 820, TitleGap: 90,
		RuleY: 417, KeyLabelY: 467, KeyY: 522, KeySize: 48, KeyGap: 58, KeyDigits: 32,
	},
	"square": {
		Width: 1080, Height: 1080, Margin: 64, BrandY: 120, BrandSize: 48,
		IconX: 64, IconY: 210, IconSize: 188, RoleX: 296, RoleY: 323, RoleSize: 48,
		TitleY: 485, TitleSize: 88, TitleWidth: 952, TitleGap: 98,
		RuleY: 630, KeyLabelY: 695, KeyY: 774, KeySize: 72, KeyGap: 82, KeyDigits: 16,
	},
	"portrait": {
		Width: 1080, Height: 1350, Margin: 64, BrandY: 144, BrandSize: 48,
		IconX: 64, IconY: 260, IconSize: 232, RoleX: 344, RoleY: 395, RoleSize: 48,
		TitleY: 630, TitleSize: 92, TitleWidth: 952, TitleGap: 110,
		RuleY: 850, KeyLabelY: 916, KeyY: 1000, KeySize: 72, KeyGap: 82, KeyDigits: 16,
	},
	"story": {
		Width: 1080, Height: 1920, Margin: 64, BrandY: 260, BrandSize: 48,
		IconX: 64, IconY: 430, IconSize: 260, RoleX: 372, RoleY: 580, RoleSize: 48,
		TitleY: 880, TitleSize: 96, TitleWidth: 952, TitleGap: 112,
		RuleY: 1150, KeyLabelY: 1230, KeyY: 1320, KeySize: 72, KeyGap: 82, KeyDigits: 16,
	},
}

func renderShareCard(site shareSite, p sharePreview, layout shareCardLayout) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, layout.Width, layout.Height))
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
	rect(0, 0, layout.Width, layout.Height, bg)
	rect(0, 0, layout.Width*3/4, 6, teal)
	rect(layout.Width*3/4, 0, layout.Width/4, 6, gold)
	type faceKey struct {
		size int
		font *opentype.Font
	}
	faces := map[faceKey]font.Face{}
	face := func(size int, fnt *opentype.Font) font.Face {
		key := faceKey{size, fnt}
		if f, ok := faces[key]; ok {
			return f
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
	// Decode only the emoji used by this request; discard them after rendering.
	emojiCache := map[string]image.Image{}
	var textErr error
	text := func(s string, x, y int, f font.Face, c color.RGBA) {
		if textErr == nil {
			textErr = drawShareCardText(img, s, x, y, f, c, emojiCache)
		}
	}
	contentWidth := layout.Width - 2*layout.Margin
	// The small mountain is the brand; the key-derived identicon identifies the node.
	poly := func(points [][2]float32, c color.RGBA) {
		z := vector.NewRasterizer(layout.Width, layout.Height)
		for i, p := range points {
			x, y := p[0]+float32(layout.Margin), p[1]+float32(layout.BrandY-53)
			if i == 0 {
				z.MoveTo(x, y)
			} else {
				z.LineTo(x, y)
			}
		}
		z.ClosePath()
		z.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
	}
	poly([][2]float32{{0, 43}, {20, 16}, {35, 29}, {58, 0}, {85, 43}, {75, 48}, {57, 19}, {37, 46}, {22, 32}, {9, 50}}, teal)
	poly([][2]float32{{8, 60}, {25, 40}, {39, 52}, {58, 31}, {81, 61}, {74, 67}, {57, 46}, {40, 67}, {26, 55}, {14, 67}}, gold)
	brandFace := face(layout.BrandSize, cardBold)
	brandWidth := contentWidth - 112
	if layout.KeyDigits == 32 {
		brandWidth = layout.IconX - layout.Margin - 168
	}
	text(shareCardLines(site.Name, brandFace, brandWidth, 1)[0], layout.Margin+112, layout.BrandY, brandFace, white)
	if p.Node != nil {
		roleFace := face(layout.RoleSize, cardRegular)
		text(shareCardLines(shareRole(p.Node.Role), roleFace, layout.Width-layout.Margin-layout.RoleX, 1)[0], layout.RoleX, layout.RoleY, roleFace, teal)
	}
	titleFace := face(layout.TitleSize, cardBold)
	titleRows := shareCardLines(cleanShareText(p.Title, 100), titleFace, layout.TitleWidth, 2)
	for i, row := range titleRows {
		text(row, layout.Margin, layout.TitleY+i*layout.TitleGap, titleFace, white)
	}
	identity := p.Path
	if p.Node != nil {
		identity = p.Node.PublicKey
	}
	sum := sha256.Sum256([]byte(identity))
	iconY := layout.IconY
	if layout.KeyDigits == 32 {
		// Align the avatar with the actual role/title ink, not empty title rows.
		firstTop, _ := shareCardTextVerticalBounds(titleRows[0], titleFace)
		_, lastBottom := shareCardTextVerticalBounds(titleRows[len(titleRows)-1], titleFace)
		top := layout.TitleY + firstTop
		bottom := layout.TitleY + (len(titleRows)-1)*layout.TitleGap + lastBottom
		if p.Node != nil {
			roleBounds, _ := font.BoundString(face(layout.RoleSize, cardRegular), shareRole(p.Node.Role))
			top = layout.RoleY + roleBounds.Min.Y.Floor()
		}
		iconY = (top + bottom - layout.IconSize) / 2
	}
	rect(layout.IconX, iconY, layout.IconSize, layout.IconSize, panel)
	cell := layout.IconSize * 4 / 25
	inset := (layout.IconSize - cell*5) / 2
	// Vertically center the visible pattern even when its outer rows are blank.
	minRow, maxRow := 4, 0
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			if sum[y*3+x]&1 != 0 {
				minRow = min(minRow, y)
				maxRow = max(maxRow, y)
			}
		}
	}
	if minRow > maxRow {
		minRow, maxRow = 0, 4
	}
	patternY := iconY + (layout.IconSize-((maxRow-minRow)*cell+cell*4/5))/2
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			if sum[y*3+x]&1 != 0 {
				rect(layout.IconX+inset+x*cell, patternY+(y-minRow)*cell, cell*4/5, cell*4/5, teal)
				rect(layout.IconX+inset+(4-x)*cell, patternY+(y-minRow)*cell, cell*4/5, cell*4/5, teal)
			}
		}
	}
	rect(layout.Margin, layout.RuleY, contentWidth, 1, line)
	if p.Node != nil {
		text("Public key", layout.Margin, layout.KeyLabelY, face(40, cardRegular), muted)
		// Every digit is rendered. Keys never pass through text truncation.
		for i, row := range shareKeyLines(p.Node.PublicKey, layout.KeyDigits) {
			text(row, layout.Margin, layout.KeyY+i*layout.KeyGap, face(layout.KeySize, cardMono), white)
		}
	} else {
		size, gap, maxLines := 48, 64, 4
		if layout.KeyDigits == 32 {
			size, gap, maxLines = 40, 54, 2
		}
		f := face(size, cardRegular)
		for i, row := range shareCardLines(p.Description, f, contentWidth, maxLines) {
			text(row, layout.Margin, layout.KeyLabelY+i*gap, f, muted)
		}
	}
	if textErr != nil {
		return nil, textErr
	}
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}

// Wrap at word boundaries when possible, retaining a readable fixed font size.
// Only names/descriptions may ellipsize; the full Unicode name remains in HTML.
func shareCardLines(s string, f font.Face, width, maxLines int) []string {
	var lines []string
	for len(lines) < maxLines {
		r := shareCardClusters(strings.TrimSpace(s), f)
		if shareCardTextWidth(strings.Join(r, ""), f).Ceil() <= width {
			return append(lines, strings.Join(r, ""))
		}
		last := len(lines) == maxLines-1
		suffix := ""
		if last {
			suffix = "…"
		}
		cut := len(r)
		for cut > 0 && shareCardTextWidth(strings.Join(r[:cut], "")+suffix, f).Ceil() > width {
			cut--
		}
		if !last {
			for i := cut; i > cut/2; i-- {
				if r[i-1] == " " {
					cut = i - 1
					break
				}
			}
		}
		lines = append(lines, strings.Join(r[:cut], "")+suffix)
		s = strings.Join(r[cut:], "")
	}
	return lines
}

// lookupPreview validates a 64-character hex key before rendering. Eight-digit
// groups wrap into two landscape rows or four larger rows for taller formats.
func shareKeyLines(key string, digitsPerLine int) []string {
	var lines []string
	for start := 0; start < len(key); start += digitsPerLine {
		var groups []string
		for i := start; i < start+digitsPerLine && i < len(key); i += 8 {
			groups = append(groups, key[i:min(i+8, len(key))])
		}
		lines = append(lines, strings.Join(groups, " "))
	}
	return lines
}
