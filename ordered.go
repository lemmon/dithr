package dithr

import (
	"image"
	"image/color"
	"runtime"
	"sort"
	"sync"

	"github.com/makeworld-the-better-one/dither/v2"
)

// Ordered dithering for arbitrary palettes.
//
// A plain "add a threshold offset, then pick the nearest colour" ditherer
// only works when palette colours are evenly spaced. Real palettes have gaps
// (e.g. black, then nothing until a mid brown), and dark areas end up
// speckled far too bright.
//
// Instead this uses Thomas Knoll's pattern dithering: for each pixel, build a
// list of K palette colours whose average (in linear light) approximates the
// pixel, by repeatedly picking the nearest colour to "target + accumulated
// error". Sort the list by lightness and let the threshold map choose one
// entry. Neighbouring pixels then show different entries of the same mix.

type thresholdMap struct {
	w, h int
	t    []float64 // thresholds in (0,1)
}

// orderedMap returns the threshold map for an ordered dither name, or nil
// for error diffusion and "none".
func orderedMap(name string) *thresholdMap {
	if n, ok := bayerSizes[name]; ok {
		t := bayerMap(int(n))
		return &t
	}
	if odm, ok := orderedMatrices[name]; ok {
		t := matrixMap(odm)
		return &t
	}
	return nil
}

func (m thresholdMap) at(x, y int) float64 { return m.t[(y%m.h)*m.w+x%m.w] }

func bayerMap(n int) thresholdMap {
	m := [][]int{{0}}
	for size := 1; size < n; size *= 2 {
		next := make([][]int, size*2)
		for y := range next {
			next[y] = make([]int, size*2)
		}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				v := 4 * m[y][x]
				next[y][x] = v
				next[y][x+size] = v + 2
				next[y+size][x] = v + 3
				next[y+size][x+size] = v + 1
			}
		}
		m = next
	}
	return newThresholdMap(m, n*n)
}

func matrixMap(odm dither.OrderedDitherMatrix) thresholdMap {
	m := make([][]int, len(odm.Matrix))
	for y, row := range odm.Matrix {
		m[y] = make([]int, len(row))
		for x, v := range row {
			m[y][x] = int(v)
		}
	}
	return newThresholdMap(m, int(odm.Max))
}

func newThresholdMap(m [][]int, max int) thresholdMap {
	tm := thresholdMap{w: len(m[0]), h: len(m)}
	for _, row := range m {
		for _, v := range row {
			tm.t = append(tm.t, (float64(v)+0.5)/float64(max))
		}
	}
	return tm
}

type palColor struct {
	lin [3]float64 // linear RGB
	lab lab
}

// maxCandidates caps the mix list per pixel. 64 = every level of an 8×8
// Bayer matrix; more would be slow for no visible gain.
const maxCandidates = 64

// orderedDither dithers b to pal. With tm == nil it does plain nearest-colour
// mapping. strength 1 = full dither, toward 0 = flatter / more posterized.
//
// alphaShared is set when the alpha channel is dithered with the same
// threshold map (transparent "dither" mode). A pixel is kept when t < alpha,
// so kept pixels only ever see low thresholds, which pick the dark end of
// the candidate list. Rescaling t to t/alpha spreads them over the whole list
// again, so a fade keeps its colour mix instead of turning dark.
func orderedDither(b *buffer, pal []color.RGBA, tm *thresholdMap, strength float64, alphaShared bool) *image.Paletted {
	pc := make([]palColor, len(pal))
	cp := make(color.Palette, len(pal))
	for i, c := range pal {
		r := fromRGBA(c)
		pc[i] = palColor{[3]float64{srgbToLinear(r.R), srgbToLinear(r.G), srgbToLinear(r.B)}, toOklab(r)}
		cp[i] = c
	}
	img := image.NewPaletted(image.Rect(0, 0, b.w, b.h), cp)

	k := 1
	if tm != nil && strength > 0 {
		k = min(len(tm.t), maxCandidates)
	}

	rows := make(chan int, b.h)
	for y := 0; y < b.h; y++ {
		rows <- y
	}
	close(rows)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cand := make([]int, k)
			for y := range rows {
				for x := 0; x < b.w; x++ {
					i := y*b.w + x
					p := b.px[i]
					goal := [3]float64{srgbToLinear(p.R), srgbToLinear(p.G), srgbToLinear(p.B)}
					var errAcc [3]float64
					for c := 0; c < k; c++ {
						var t [3]float64
						for ch := 0; ch < 3; ch++ {
							t[ch] = goal[ch] + errAcc[ch]*strength
						}
						j := nearest(pc, oklabFromLinear(t[0], t[1], t[2]))
						cand[c] = j
						for ch := 0; ch < 3; ch++ {
							errAcc[ch] += goal[ch] - pc[j].lin[ch]
						}
					}
					if k == 1 {
						img.Pix[i] = uint8(cand[0])
						continue
					}
					sort.Slice(cand, func(m, n int) bool { return pc[cand[m]].lab.L < pc[cand[n]].lab.L })
					t := tm.at(x, y)
					if a := b.alpha[i]; alphaShared && a > 0 && t < a {
						t /= a
					}
					img.Pix[i] = uint8(cand[int(t*float64(k))])
				}
			}
		}()
	}
	wg.Wait()
	return img
}

func nearest(pc []palColor, c lab) int {
	bi, bd := 0, labDist(c, pc[0].lab)
	for j := 1; j < len(pc); j++ {
		if d := labDist(c, pc[j].lab); d < bd {
			bi, bd = j, d
		}
	}
	return bi
}
