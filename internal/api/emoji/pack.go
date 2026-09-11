//go:build ignore

// Called by update.py: normalize large flag sources onto a 128px transparent
// square, then keep all PNGs and upstream notices in a reproducible archive.
package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

func main() {
	source, err := zip.OpenReader(os.Args[1])
	must(err)
	defer source.Close()
	file, err := os.Create(os.Args[2])
	must(err)
	out := zip.NewWriter(file)
	for _, entry := range source.File {
		r, err := entry.Open()
		must(err)
		data, err := io.ReadAll(r)
		r.Close()
		must(err)
		if strings.HasSuffix(entry.Name, ".png") {
			cfg, err := png.DecodeConfig(bytes.NewReader(data))
			must(err)
			if cfg.Width != 128 || cfg.Height != 128 {
				if cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
					panic("oversized upstream flag")
				}
				img, err := png.Decode(bytes.NewReader(data))
				must(err)
				w, h := 128, 128*cfg.Height/cfg.Width
				if cfg.Height > cfg.Width {
					w, h = 128*cfg.Width/cfg.Height, 128
				}
				canvas := image.NewNRGBA(image.Rect(0, 0, 128, 128))
				x, y := (128-w)/2, (128-h)/2
				draw.CatmullRom.Scale(canvas, image.Rect(x, y, x+w, y+h), img, img.Bounds(), draw.Over, nil)
				var b bytes.Buffer
				encoder := png.Encoder{CompressionLevel: png.BestCompression}
				must(encoder.Encode(&b, canvas))
				data = b.Bytes()
			}
		}
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		header.SetMode(0644)
		w, err := out.CreateHeader(header)
		must(err)
		_, err = w.Write(data)
		must(err)
	}
	must(out.Close())
	must(file.Close())
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
