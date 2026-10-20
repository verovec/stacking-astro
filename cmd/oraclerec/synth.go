package main

// Synthetic captures of the two live goldens, copied from internal/pipeline/golden_live_test.go
// (writeGoldenFrame, buildGoldenCapture, goldenLCG) and golden_multinight_live_test.go
// (writeGoldenFrameRot, buildTwoNightCapture): those live in _test files and take *testing.T, so
// they cannot be imported. The pixel streams are kept verbatim — a change here must be mirrored
// there or case (a) stops matching the pinned run.json. Unlike fitstest, header cards are written
// in a fixed (sorted) order, so regenerated frames are byte-identical, not just pixel-identical.

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type goldenLCG struct{ s uint64 }

func (l *goldenLCG) next() float64 {
	l.s = l.s*6364136223846793005 + 1442695040888963407
	return float64(l.s>>11) / float64(1<<53)
}

const synthW, synthH = 256, 256

func writeGoldenFrame(dir, name, imagetyp, filter string, expSec float64, seq int) error {
	const w, h = synthW, synthH
	rnd := goldenLCG{s: uint64(1e6*expSec) + uint64(seq)<<32 + uint64(len(filter))}
	grid := make([]float64, w*h)
	for i := range grid {
		grid[i] = 500 + 40*rnd.next()
	}
	switch imagetyp {
	case "Light":
		dx, dy := float64(seq)*1.5, float64(seq)*0.5
		stars := goldenLCG{s: 42}
		for s := 0; s < 40; s++ {
			cx := 20 + stars.next()*float64(w-40) + dx
			cy := 20 + stars.next()*float64(h-40) + dy
			amp := 8000 + 24000*stars.next()
			addStar(grid, cx, cy, amp)
		}
	case "Flat":
		vignette(grid)
	case "Dark":
		for i := range grid {
			grid[i] += 20
		}
	}
	return writeFrame(dir, name, grid, goldenCards(imagetyp, filter, expSec, seq, "2024-03-01"))
}

func writeGoldenFrameRot(dir, name, imagetyp, filter string, expSec float64, seq int,
	rotDeg float64, night string, pedestal, spread float64) error {
	const w, h = synthW, synthH
	rnd := goldenLCG{s: uint64(1e6*expSec) + uint64(seq)<<32 + uint64(len(filter))*7 + 99}
	grid := make([]float64, w*h)
	for i := range grid {
		grid[i] = pedestal + spread*rnd.next()
	}
	switch imagetyp {
	case "Light":
		rad := rotDeg * math.Pi / 180
		cos, sin := math.Cos(rad), math.Sin(rad)
		dx, dy := float64(seq)*1.2, float64(seq)*-0.8
		stars := goldenLCG{s: 42}
		for s := 0; s < 40; s++ {
			bx := 20 + stars.next()*float64(w-40)
			by := 20 + stars.next()*float64(h-40)
			amp := 8000 + 24000*stars.next()
			cx := (bx-w/2)*cos - (by-h/2)*sin + w/2 + dx
			cy := (bx-w/2)*sin + (by-h/2)*cos + h/2 + dy
			addStar(grid, cx, cy, amp)
		}
	case "Flat":
		vignette(grid)
	case "Dark":
		for i := range grid {
			grid[i] += 20
		}
	}
	return writeFrame(dir, name, grid, goldenCards(imagetyp, filter, expSec, seq, night))
}

// addStar and vignette keep the exact arithmetic order of the golden loops.
func addStar(grid []float64, cx, cy, amp float64) {
	const w, h = synthW, synthH
	for y := int(cy) - 6; y <= int(cy)+6; y++ {
		for x := int(cx) - 6; x <= int(cx)+6; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			d2 := (float64(x)-cx)*(float64(x)-cx) + (float64(y)-cy)*(float64(y)-cy)
			grid[y*w+x] += amp * math.Exp(-d2/(2*2.1*2.1))
		}
	}
}

func vignette(grid []float64) {
	const w, h = synthW, synthH
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x-w/2)/float64(w), float64(y-h/2)/float64(h)
			grid[y*w+x] = 30000 - 8000*(fx*fx+fy*fy)*4
		}
	}
}

func goldenCards(imagetyp, filter string, expSec float64, seq int, night string) map[string]string {
	cards := map[string]string{
		"OBJECT": "'TESTFIELD'", "IMAGETYP": "'" + imagetyp + "'",
		"EXPTIME": fmt.Sprintf("%g", expSec), "GAIN": "100", "OFFSET": "10",
		"CCD-TEMP": "-10.0", "XBINNING": "1", "YBINNING": "1",
		"DATE-OBS": fmt.Sprintf("'%sT22:%02d:%02d'", night, 10+seq, (seq*7)%60),
		"INSTRUME": "'ZWO ASI1600MM Pro'",
	}
	if filter != "" {
		cards["FILTER"] = "'" + filter + "'"
	}
	return cards
}

// buildGoldenCapture: case (a), 3 lights × L/R/G/B + 3 darks/flats/bias, one night.
func buildGoldenCapture(root string) error {
	for _, f := range []string{"L", "R", "G", "B"} {
		for i := 0; i < 3; i++ {
			if err := writeGoldenFrame(filepath.Join(root, "lights"), fmt.Sprintf("light_%s_%d", f, i), "Light", f, 10, i); err != nil {
				return err
			}
		}
	}
	for i := 0; i < 3; i++ {
		for _, fr := range []struct {
			dir, name, typ string
			exp            float64
		}{
			{"darks", "dark", "Dark", 10}, {"flats", "flat", "Flat", 0.005}, {"bias", "bias", "Bias", 0.0001},
		} {
			if err := writeGoldenFrame(filepath.Join(root, fr.dir), fmt.Sprintf("%s_%d", fr.name, i), fr.typ, "", fr.exp, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildTwoNightCapture: case (b), the (a) capture plus night 2020-05-15 (L/R, 30 s, rotated).
func buildTwoNightCapture(root string, rotDeg float64) error {
	if err := buildGoldenCapture(root); err != nil {
		return err
	}
	const nightB = "2020-05-15"
	for _, f := range []string{"L", "R"} {
		for i := 0; i < 2; i++ {
			if err := writeGoldenFrameRot(filepath.Join(root, "lights_b"), fmt.Sprintf("light_b_%s_%d", f, i),
				"Light", f, 30, i, rotDeg, nightB, 520, 40); err != nil {
				return err
			}
		}
	}
	for i := 0; i < 3; i++ {
		if err := writeGoldenFrameRot(filepath.Join(root, "darks_b"), fmt.Sprintf("dark_b_%d", i), "Dark", "", 30, i, 0, nightB, 500, 40); err != nil {
			return err
		}
		if err := writeGoldenFrameRot(filepath.Join(root, "flats_b"), fmt.Sprintf("flat_b_%d", i), "Flat", "", 0.005, i, 0, nightB, 500, 40); err != nil {
			return err
		}
	}
	return nil
}

// writeFrame clamps the grid to uint16 and writes a 16-bit FITS (BZERO 32768) exactly as
// fitstest.WritePixels does, except that the extra cards come in sorted order.
func writeFrame(dir, name string, grid []float64, cards map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data := make([]byte, len(grid)*2)
	for i, v := range grid {
		if v < 0 {
			v = 0
		}
		if v > 65535 {
			v = 65535
		}
		binary.BigEndian.PutUint16(data[i*2:], uint16(int32(uint16(v))-32768))
	}
	for len(data)%fitsBlock != 0 {
		data = append(data, 0)
	}
	hdr := fitsHeader(synthW, synthH, cards)
	return os.WriteFile(filepath.Join(dir, name+".fits"), append(hdr, data...), 0o644)
}

func fitsHeader(w, h int, cards map[string]string) []byte {
	var hdr []byte
	add := func(key, val string) {
		s := fmt.Sprintf("%-8s= %s", key, val)
		if len(s) > 80 {
			s = s[:80]
		}
		hdr = append(hdr, s+strings.Repeat(" ", 80-len(s))...)
	}
	add("SIMPLE", "T")
	add("BITPIX", "16")
	add("NAXIS", "2")
	add("NAXIS1", strconv.Itoa(w))
	add("NAXIS2", strconv.Itoa(h))
	add("BZERO", "32768")
	add("BSCALE", "1")
	keys := make([]string, 0, len(cards))
	for k := range cards {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(k, cards[k])
	}
	hdr = append(hdr, "END"+strings.Repeat(" ", 77)...)
	for len(hdr)%fitsBlock != 0 {
		hdr = append(hdr, ' ')
	}
	return hdr
}
