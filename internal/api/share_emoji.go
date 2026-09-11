package api

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// The archive includes Noto PNGs, normalized flags and upstream license/notices.
//
//go:embed emoji/noto-emoji.zip
var shareEmojiArchive []byte

type shareEmojiTrie struct {
	next map[rune]*shareEmojiTrie
	file *zip.File
}

var shareEmojiRoot = func() *shareEmojiTrie {
	archive, err := zip.NewReader(bytes.NewReader(shareEmojiArchive), int64(len(shareEmojiArchive)))
	if err != nil {
		panic(err)
	}
	root := &shareEmojiTrie{next: map[rune]*shareEmojiTrie{}}
	files := map[string]*zip.File{}
	insert := func(sequence string, file *zip.File) {
		cur := root
		var codepoints []rune
		for _, hex := range strings.Split(sequence, "_") {
			n, err := strconv.ParseInt(hex, 16, 32)
			if err != nil {
				panic(err)
			}
			r := rune(n)
			if r == '\ufe0f' {
				continue
			}
			codepoints = append(codepoints, r)
			if cur.next[r] == nil {
				cur.next[r] = &shareEmojiTrie{next: map[rune]*shareEmojiTrie{}}
			}
			cur = cur.next[r]
		}
		// Noto also contains fallback glyphs for plain digits and isolated
		// regional indicators. Those are text, not standalone emoji artwork.
		if len(codepoints) == 0 {
			return
		}
		if len(codepoints) == 1 {
			r := codepoints[0]
			if r < 0x80 || (r >= 0x1f1e6 && r <= 0x1f1ff) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return
			}
		}
		cur.file = file
	}
	for _, f := range archive.File {
		if strings.HasPrefix(f.Name, "emoji_u") && strings.HasSuffix(f.Name, ".png") {
			seq := strings.TrimSuffix(strings.TrimPrefix(f.Name, "emoji_u"), ".png")
			files[seq] = f
			insert(seq, f)
		}
	}
	// Upstream aliases cover equivalent gender/ZWJ/legacy sequences that share art.
	for _, f := range archive.File {
		if f.Name != "emoji_aliases.txt" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			panic(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			parts := strings.Split(line, ";")
			if len(parts) != 2 {
				continue
			}
			if target := files[strings.TrimSpace(parts[1])]; target != nil {
				insert(strings.TrimSpace(parts[0]), target)
			}
		}
	}
	return root
}()

// Match a complete known sequence, preferring the longest (flags, skin tones,
// keycaps and ZWJ families/professions). VS16 selects emoji; VS15 requests text.
func shareEmojiMatch(s string) (*zip.File, int) {
	cur := shareEmojiRoot
	var best *zip.File
	end := 0
	for i, r := range s {
		if r == '\ufe0e' {
			return nil, 0
		}
		if r != '\ufe0f' || cur == shareEmojiRoot {
			cur = cur.next[r]
			if cur == nil {
				break
			}
		}
		if cur.file != nil {
			best, end = cur.file, i+utf8.RuneLen(r)
		}
	}
	return best, end
}

func nextShareCluster(s string) (string, *zip.File) {
	if f, end := shareEmojiMatch(s); f != nil {
		return s[:end], f
	}
	_, size := utf8.DecodeRuneInString(s)
	return s[:size], nil
}

func shareCardClusters(s string, f font.Face) []string {
	var clusters []string
	for s != "" {
		cluster, emoji := nextShareCluster(s)
		s = s[len(cluster):]
		if emoji == nil {
			r, _ := utf8.DecodeRuneInString(cluster)
			if r == '\ufe0e' || r == '\ufe0f' {
				continue
			}
			if _, ok := f.GlyphAdvance(r); !ok {
				cluster = "□"
			}
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

func shareEmojiSize(f font.Face) int {
	return (f.Metrics().Ascent + f.Metrics().Descent).Ceil() * 4 / 5
}

func shareCardTextWidth(s string, f font.Face) fixed.Int26_6 {
	var width fixed.Int26_6
	var run strings.Builder
	for s != "" {
		cluster, emoji := nextShareCluster(s)
		s = s[len(cluster):]
		if emoji == nil {
			run.WriteString(cluster)
			continue
		}
		width += font.MeasureString(f, run.String()) + fixed.I(shareEmojiSize(f)*11/10)
		run.Reset()
	}
	return width + font.MeasureString(f, run.String())
}

func drawShareCardText(dst *image.RGBA, s string, x, y int, f font.Face, c color.RGBA, cache map[string]image.Image) error {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	var run strings.Builder
	for s != "" {
		cluster, emoji := nextShareCluster(s)
		s = s[len(cluster):]
		if emoji == nil {
			run.WriteString(cluster)
			continue
		}
		d.DrawString(run.String())
		run.Reset()
		art := cache[emoji.Name]
		if art == nil {
			r, err := emoji.Open()
			if err != nil {
				return err
			}
			art, err = png.Decode(r)
			r.Close()
			if err != nil {
				return fmt.Errorf("decode bundled emoji %s: %w", emoji.Name, err)
			}
			cache[emoji.Name] = art
		}
		size := shareEmojiSize(f)
		left, top := d.Dot.X.Round(), y-size*4/5
		xdraw.CatmullRom.Scale(dst, image.Rect(left, top, left+size, top+size), art, art.Bounds(), xdraw.Over, nil)
		d.Dot.X += fixed.I(size * 11 / 10)
	}
	d.DrawString(run.String())
	return nil
}

// Relative ink bounds, including the square used for color emoji artwork.
func shareCardTextVerticalBounds(s string, f font.Face) (int, int) {
	bounds, _ := font.BoundString(f, s)
	top, bottom := bounds.Min.Y.Floor(), bounds.Max.Y.Ceil()
	for s != "" {
		cluster, emoji := nextShareCluster(s)
		s = s[len(cluster):]
		if emoji != nil {
			size := shareEmojiSize(f)
			return min(top, -size*4/5), max(bottom, size-size*4/5)
		}
	}
	return top, bottom
}
