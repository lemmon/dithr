package dithr

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Built-in palettes. "gray" is generated from Options.Colors.
var builtinPalettes = map[string][]string{
	"bw":      {"000000", "ffffff"},
	"gameboy": {"0f380f", "306230", "8bac0f", "9bbc0f"},
	"cga":     {"000000", "55ffff", "ff55ff", "ffffff"},
	"ega": {
		"000000", "0000aa", "00aa00", "00aaaa", "aa0000", "aa00aa", "aa5500", "aaaaaa",
		"555555", "5555ff", "55ff55", "55ffff", "ff5555", "ff55ff", "ffff55", "ffffff",
	},
	"c64": {
		"000000", "ffffff", "68372b", "70a4b2", "6f3d86", "588d43", "352879", "b8c76f",
		"6f4f25", "433900", "9a6759", "444444", "6c6c6c", "9ad284", "6c5eb5", "959595",
	},
	"db16": { // DawnBringer 16, a classic pixel-art palette
		"140c1c", "442434", "30346d", "4e4a4e", "854c30", "346524", "d04648", "757161",
		"597dce", "d27d2c", "8595a1", "6daa2c", "d2aa99", "6dc2ca", "dad45e", "deeed6",
	},
	"win16": { // Windows 3.1 / VGA default 16 colours
		"000000", "800000", "008000", "808000", "000080", "800080", "008080", "c0c0c0",
		"808080", "ff0000", "00ff00", "ffff00", "0000ff", "ff00ff", "00ffff", "ffffff",
	},
	"amber":     {"0d0700", "3a1f00", "8a4f00", "d98a00", "ffc34d"},           // amber CRT monitor
	"phosphor":  {"000d02", "053d12", "0d8a2a", "33ff66", "c4ffd4"},           // green CRT monitor
	"sepia":     {"1b1310", "4a3222", "7d5638", "b08455", "dcc095", "f4e8cc"}, // old adventure game / book print
	"cyanotype": {"08162e", "123a6b", "2563a8", "6fa3d9", "c7def2", "f2f8ff"},
	"synthwave": {"0d0221", "261447", "541388", "a3166b", "f0427a", "ff9e5e", "ffd319"},
	"pico8": {
		"000000", "1d2b53", "7e2553", "008751", "ab5236", "5f574f", "c2c3c7", "fff1e8",
		"ff004d", "ffa300", "ffec27", "00e436", "29adff", "83769c", "ff77a8", "ffccaa",
	},
}

// PaletteNames lists the built-in palette names (plus "original" and "gray").
func PaletteNames() []string {
	names := []string{"original", "gray"}
	for k := range builtinPalettes {
		names = append(names, k)
	}
	sort.Strings(names[2:])
	return names
}

// ParseHexColor parses "#rgb", "#rrggbb" (the # is optional), "black" or "white".
func ParseHexColor(s string) (color.RGBA, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "black":
		return color.RGBA{0, 0, 0, 255}, nil
	case "white":
		return color.RGBA{255, 255, 255, 255}, nil
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid colour %q (use #rgb or #rrggbb)", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid colour %q (use #rgb or #rrggbb)", s)
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

// ParsePalette turns a palette spec into colours. The spec is a built-in name
// ("bw", "gray", "gameboy", "cga", "pico8") or a list of hex colours separated
// by commas, spaces or newlines (a Lospec .hex file's contents work too).
// "original" (or "") returns nil, meaning: extract colours from the image.
func ParsePalette(spec string, colors int) ([]color.RGBA, error) {
	name := strings.ToLower(strings.TrimSpace(spec))
	if name == "" || name == "original" {
		return nil, nil
	}
	if name == "gray" || name == "grey" {
		if colors < 2 {
			return nil, fmt.Errorf("gray palette needs at least 2 colours")
		}
		pal := make([]color.RGBA, colors)
		for i := range pal {
			v := uint8(math.Round(float64(i) / float64(colors-1) * 255))
			pal[i] = color.RGBA{v, v, v, 255}
		}
		return pal, nil
	}
	if hexes, ok := builtinPalettes[name]; ok {
		spec = strings.Join(hexes, ",")
	}
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	var pal []color.RGBA
	for _, f := range fields {
		c, err := ParseHexColor(f)
		if err != nil {
			return nil, fmt.Errorf("palette: %w (built-ins: %s)", err, strings.Join(PaletteNames(), ", "))
		}
		pal = append(pal, c)
	}
	if len(pal) == 0 {
		return nil, fmt.Errorf("palette %q has no colours", spec)
	}
	return pal, nil
}

// HexList formats a palette as "#rrggbb,#rrggbb,..." so it can be reused with --palette.
func HexList(pal []color.RGBA) string {
	parts := make([]string, len(pal))
	for i, c := range pal {
		parts[i] = fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return strings.Join(parts, ",")
}

// extractPalette picks n representative colours from the buffer using median
// cut followed by a few k-means passes, both in OKLab. Pixels count in
// proportion to weight, how much of them ends up visible (a pixel shown 30%
// of the time is sampled 30% of the time), so the background doesn't eat
// colours but faint, still-visible areas get theirs.
func extractPalette(b *buffer, weight []float64, n int) []color.RGBA {
	const maxSamples = 80000
	var idx []int
	for i, a := range weight {
		// deterministic "random" acceptance: golden-ratio sequence in 0..1
		if _, frac := math.Modf(float64(i) * 0.6180339887498949); a > frac {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		idx = make([]int, len(b.px))
		for i := range idx {
			idx[i] = i
		}
	}
	step := len(idx)/maxSamples + 1
	pts := make([]lab, 0, len(idx)/step+1)
	for i := 0; i < len(idx); i += step {
		pts = append(pts, toOklab(b.px[idx[i]]))
	}

	centers := medianCut(pts, n)
	centers = kmeans(pts, centers, 8)

	sort.Slice(centers, func(i, j int) bool { return centers[i].L < centers[j].L })
	pal := make([]color.RGBA, 0, len(centers))
	seen := map[color.RGBA]bool{}
	for _, c := range centers {
		rc := toRGBA(fromOklab(c))
		if !seen[rc] {
			seen[rc] = true
			pal = append(pal, rc)
		}
	}
	return pal
}

func axis(p lab, a int) float64 {
	switch a {
	case 0:
		return p.L
	case 1:
		return p.A
	}
	return p.B
}

func medianCut(pts []lab, n int) []lab {
	boxes := [][]lab{pts}
	for len(boxes) < n {
		// Split the box with the biggest spread, weighted by sqrt(pixel count) so
		// small but distinct areas (a patch of sky) still get their own colour.
		best, bestAxis, bestScore := -1, 0, 0.0
		for i, bx := range boxes {
			if len(bx) < 2 {
				continue
			}
			for a := 0; a < 3; a++ {
				lo, hi := axis(bx[0], a), axis(bx[0], a)
				for _, p := range bx {
					v := axis(p, a)
					if v < lo {
						lo = v
					}
					if v > hi {
						hi = v
					}
				}
				if score := (hi - lo) * math.Sqrt(float64(len(bx))); score > bestScore {
					best, bestAxis, bestScore = i, a, score
				}
			}
		}
		if best < 0 {
			break // nothing left to split
		}
		bx := boxes[best]
		sort.Slice(bx, func(i, j int) bool { return axis(bx[i], bestAxis) < axis(bx[j], bestAxis) })
		mid := len(bx) / 2
		boxes[best] = bx[:mid]
		boxes = append(boxes, bx[mid:])
	}
	centers := make([]lab, len(boxes))
	for i, bx := range boxes {
		centers[i] = mean(bx)
	}
	return centers
}

func mean(pts []lab) lab {
	var s lab
	for _, p := range pts {
		s.L += p.L
		s.A += p.A
		s.B += p.B
	}
	n := float64(len(pts))
	return lab{s.L / n, s.A / n, s.B / n}
}

func kmeans(pts []lab, centers []lab, iters int) []lab {
	for it := 0; it < iters; it++ {
		sums := make([]lab, len(centers))
		counts := make([]int, len(centers))
		for _, p := range pts {
			bi, bd := 0, labDist(p, centers[0])
			for j := 1; j < len(centers); j++ {
				if d := labDist(p, centers[j]); d < bd {
					bi, bd = j, d
				}
			}
			sums[bi].L += p.L
			sums[bi].A += p.A
			sums[bi].B += p.B
			counts[bi]++
		}
		for j := range centers {
			if counts[j] > 0 {
				n := float64(counts[j])
				centers[j] = lab{sums[j].L / n, sums[j].A / n, sums[j].B / n}
			}
		}
	}
	return centers
}

func withColor(pal []color.RGBA, c color.RGBA) []color.RGBA {
	for _, p := range pal {
		if p == c {
			return pal
		}
	}
	return append(append([]color.RGBA{}, pal...), c)
}
