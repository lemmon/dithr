package dithr

// smooth applies a Kuwahara filter: for every pixel it looks at the four
// (r+1)×(r+1) squares that have the pixel as a corner and takes the average
// colour of the most uniform one. Flat areas become flatter, edges stay
// sharp: a photo turns into something closer to a painted game background,
// so the dither shows up as clean gradients instead of noise.
//
// All sums are weighted by alpha, so colours of transparent pixels (which
// are invisible and often arbitrary) don't bleed into visible ones.
func smooth(b *buffer, r int) {
	if r <= 0 {
		return
	}
	w, h := b.w, b.h
	sw := w + 1
	// summed-area tables for R, G, B and lightness²
	satR := make([]float64, sw*(h+1))
	satG := make([]float64, sw*(h+1))
	satB := make([]float64, sw*(h+1))
	satL := make([]float64, sw*(h+1))
	satL2 := make([]float64, sw*(h+1))
	satA := make([]float64, sw*(h+1))
	for y := 0; y < h; y++ {
		var rr, rg, rb, rl, rl2, ra float64
		for x := 0; x < w; x++ {
			p, a := b.px[y*w+x], b.alpha[y*w+x]
			l := 0.299*p.R + 0.587*p.G + 0.114*p.B
			rr += a * p.R
			rg += a * p.G
			rb += a * p.B
			rl += a * l
			rl2 += a * l * l
			ra += a
			i, up := (y+1)*sw+x+1, y*sw+x+1
			satR[i] = satR[up] + rr
			satG[i] = satG[up] + rg
			satB[i] = satB[up] + rb
			satL[i] = satL[up] + rl
			satL2[i] = satL2[up] + rl2
			satA[i] = satA[up] + ra
		}
	}
	sum := func(t []float64, x0, y0, x1, y1 int) float64 { // inclusive x0..x1-1
		return t[y1*sw+x1] - t[y0*sw+x1] - t[y1*sw+x0] + t[y0*sw+x0]
	}

	out := make([]rgb, len(b.px))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			best, bestVar := b.px[y*w+x], -1.0
			for q := 0; q < 4; q++ {
				x0, y0 := x-r, y-r
				if q&1 == 1 {
					x0 = x
				}
				if q&2 == 2 {
					y0 = y
				}
				x1, y1 := min(x0+r+1, w), min(y0+r+1, h)
				x0, y0 = max(x0, 0), max(y0, 0)
				n := sum(satA, x0, y0, x1, y1) // total weight
				if n < 1e-6 {
					continue // fully transparent square
				}
				ml := sum(satL, x0, y0, x1, y1) / n
				v := sum(satL2, x0, y0, x1, y1)/n - ml*ml
				if bestVar < 0 || v < bestVar {
					bestVar = v
					best = rgb{sum(satR, x0, y0, x1, y1) / n, sum(satG, x0, y0, x1, y1) / n, sum(satB, x0, y0, x1, y1) / n}
				}
			}
			out[y*w+x] = best
		}
	}
	b.px = out
}
