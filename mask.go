package dithr

import (
	"math"
)

// buildMask multiplies b.alpha by the key and fade masks.
// Alpha 1 = keep the pixel, 0 = replace it with the background colour.
// It reports whether anything is masked at all.
func buildMask(b *buffer, o Options) bool {
	masked := false
	for _, a := range b.alpha {
		if a < 1 {
			masked = true // source image already has transparency
			break
		}
	}

	if o.Key != "none" {
		masked = true
		key := make([]float64, len(b.px))
		keyLab := toOklab(fromRGBA(o.KeyColor))
		for i, p := range b.px {
			c := toOklab(p)
			switch o.Key {
			case "light": // lightness >= cutoff becomes background
				key[i] = 1 - smoothstep(o.Cutoff-o.Feather, o.Cutoff, c.L)
			case "dark": // lightness <= cutoff becomes background
				key[i] = smoothstep(o.Cutoff, o.Cutoff+o.Feather, c.L)
			case "color": // closer than cutoff to the key colour becomes background
				key[i] = smoothstep(o.Cutoff, o.Cutoff+o.Feather, labDist(c, keyLab))
			}
		}
		if o.MaskBlur > 0 {
			boxBlur(key, b.w, b.h, o.MaskBlur)
			boxBlur(key, b.w, b.h, o.MaskBlur) // two passes ≈ gaussian
		}
		for i := range key {
			b.alpha[i] *= key[i]
		}
	}

	for _, f := range o.Fade {
		masked = true
		applyFade(b, f, o.FadeSize)
	}
	return masked
}

func applyFade(b *buffer, dir string, size float64) {
	w, h := float64(b.w), float64(b.h)
	for y := 0; y < b.h; y++ {
		for x := 0; x < b.w; x++ {
			// u, v: 0..1 across the image, measured at pixel centres
			u, v := (float64(x)+0.5)/w, (float64(y)+0.5)/h
			var k float64
			switch dir {
			case "top":
				k = smoothstep(0, size, v)
			case "bottom":
				k = smoothstep(0, size, 1-v)
			case "left":
				k = smoothstep(0, size, u)
			case "right":
				k = smoothstep(0, size, 1-u)
			case "radial":
				// ellipse touching the image edges: r = 1 at the edge midpoints
				dx, dy := u*2-1, v*2-1
				r := math.Sqrt(dx*dx + dy*dy)
				k = 1 - smoothstep(1-size, 1, r)
			}
			b.alpha[y*b.w+x] *= k
		}
	}
}

// boxBlur blurs v (w×h) in place with a box of radius r, using running sums.
func boxBlur(v []float64, w, h, r int) {
	line := make([]float64, max(w, h))
	pre := make([]float64, max(w, h)+1)
	blur := func(n int, get func(int) float64, set func(int, float64)) {
		for i := 0; i < n; i++ {
			line[i] = get(i)
			pre[i+1] = pre[i] + line[i]
		}
		for i := 0; i < n; i++ {
			lo, hi := max(i-r, 0), min(i+r+1, n)
			set(i, (pre[hi]-pre[lo])/float64(hi-lo))
		}
	}
	for y := 0; y < h; y++ {
		row := y * w
		blur(w, func(i int) float64 { return v[row+i] }, func(i int, x float64) { v[row+i] = x })
	}
	for x := 0; x < w; x++ {
		blur(h, func(i int) float64 { return v[i*w+x] }, func(i int, val float64) { v[i*w+x] = val })
	}
}
