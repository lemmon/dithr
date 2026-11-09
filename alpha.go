package dithr

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Transparent output.
//
// With Options.Transparent the keep-mask is not blended into a background
// colour. It becomes the alpha channel instead, and every output pixel is
// either fully opaque or fully transparent. DTF and DTG printers lay solid
// white ink under any pixel that isn't 100% transparent, so soft edges must be
// turned into solid dots. Three ways to do that:
//
//   threshold  hard cut at 50%
//   dither     pixel-level ordered dither (the retro dissolve, good for the web)
//   halftone   round dots on a rotated print screen, sized by LPI / DPI (for DTF)

// keepMask returns, at output resolution, which pixels stay opaque.
func keepMask(b *buffer, o Options) (keep []bool, ow, oh int) {
	p := o.Pixel
	ow, oh = b.w*p, b.h*p
	keep = make([]bool, ow*oh)

	if o.Alpha == "halftone" {
		halftoneMask(b, o, keep, ow, oh)
		return keep, ow, oh
	}

	var tm *thresholdMap
	if o.Alpha == "dither" {
		// Must be the same map the colours use: orderedDither relies on it
		// (alphaShared) to keep the colour mix right in fades.
		tm = orderedMap(o.Dither)
		if tm == nil { // error diffusion or "none": dither alpha with Bayer 8×8
			t := bayerMap(8)
			tm = &t
		}
	}
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			wx, wy := x/p, y/p // dither at working resolution, like the colours
			a := b.alpha[wy*b.w+wx]
			th := 0.5
			if tm != nil {
				th = tm.at(wx, wy)
			}
			keep[y*ow+x] = a > th
		}
	}
	return keep, ow, oh
}

func halftoneMask(b *buffer, o Options, keep []bool, ow, oh int) {
	cell := o.DPI / o.LPI // dot spacing in output px
	lut := spotThresholds(o.Dot)
	sin, cos := math.Sincos(o.Angle * math.Pi / 180)
	p := float64(o.Pixel)

	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// coverage = keep-mask value here (bilinear from working res)
			// Colours come from the nearest working pixel; if that one is fully
			// transparent its RGB is arbitrary, so it must never get a dot.
			if b.alpha[min(int(fy/p), b.h-1)*b.w+min(int(fx/p), b.w-1)] < 0.001 {
				continue
			}
			c := sampleAlpha(b, fx/p-0.5, fy/p-0.5)
			if c < o.MinDot {
				c = 0
			} else if c > 1-o.MinDot {
				c = 1
			}
			if c <= 0 {
				continue
			}
			if c >= 1 {
				keep[y*ow+x] = true
				continue
			}
			// position inside the rotated screen cell, 0..1
			u := (fx*cos + fy*sin) / cell
			v := (-fx*sin + fy*cos) / cell
			u -= math.Floor(u)
			v -= math.Floor(v)
			keep[y*ow+x] = c > lut.at(u, v)
		}
	}
}

func sampleAlpha(b *buffer, x, y float64) float64 {
	x = math.Max(0, math.Min(x, float64(b.w-1)))
	y = math.Max(0, math.Min(y, float64(b.h-1)))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, b.w-1), min(y0+1, b.h-1)
	tx, ty := x-float64(x0), y-float64(y0)
	a := b.alpha[y0*b.w+x0]*(1-tx) + b.alpha[y0*b.w+x1]*tx
	c := b.alpha[y1*b.w+x0]*(1-tx) + b.alpha[y1*b.w+x1]*tx
	return a*(1-ty) + c*ty
}

const spotRes = 64

type spotLUT []float64 // spotRes×spotRes thresholds in [0,1)

func (l spotLUT) at(u, v float64) float64 {
	return l[min(int(v*spotRes), spotRes-1)*spotRes+min(int(u*spotRes), spotRes-1)]
}

// spotThresholds turns a PostScript-style spot function into a threshold
// table. Points are ranked by spot value, so a coverage of 30% turns on
// exactly 30% of the cell, and the dot grows in the shape the function gives.
func spotThresholds(dot string) spotLUT {
	spot := func(x, y float64) float64 { // x, y in -1..1; higher = inked first
		ax, ay := math.Abs(x), math.Abs(y)
		switch dot {
		case "square":
			return 1 - math.Max(ax, ay)
		case "diamond":
			return 1 - (ax+ay)/2
		case "line":
			return 1 - ay
		default: // round (Euclidean): round dots that join into a checkerboard at 50%, then round holes
			if ax+ay <= 1 {
				return 1 - (x*x + y*y)
			}
			return (ax-1)*(ax-1) + (ay-1)*(ay-1) - 1
		}
	}
	n := spotRes * spotRes
	vals := make([]float64, n)
	order := make([]int, n)
	for i := range vals {
		u := (float64(i%spotRes) + 0.5) / spotRes
		v := (float64(i/spotRes) + 0.5) / spotRes
		vals[i] = spot(u*2-1, v*2-1)
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return vals[order[a]] > vals[order[b]] })
	lut := make(spotLUT, n)
	for rank, i := range order {
		lut[i] = (float64(rank) + 0.5) / float64(n)
	}
	return lut
}

// transparentPaletted upscales a working-res indexed image and adds a
// transparent palette entry for the pixels that aren't kept.
func transparentPaletted(src *image.Paletted, keep []bool, pixel, ow, oh int) *image.Paletted {
	pal := append(color.Palette{}, src.Palette...)
	pal = append(pal, color.NRGBA{0, 0, 0, 0})
	none := uint8(len(pal) - 1)
	dst := image.NewPaletted(image.Rect(0, 0, ow, oh), pal)
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			i := y*ow + x
			if keep[i] {
				dst.Pix[i] = src.Pix[(y/pixel)*src.Stride+x/pixel]
			} else {
				dst.Pix[i] = none
			}
		}
	}
	return dst
}

// fullColor builds an output-res image with the (unquantized) working colours.
func fullColor(b *buffer, keep []bool, pixel, ow, oh int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			i := y*ow + x
			if keep != nil && !keep[i] {
				continue // stays 0,0,0,0
			}
			c := toRGBA(b.px[(y/pixel)*b.w+x/pixel])
			copy(dst.Pix[i*4:i*4+4], []uint8{c.R, c.G, c.B, 255})
		}
	}
	return dst
}
