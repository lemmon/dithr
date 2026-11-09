// Package dithr turns images into palette-limited, dithered graphics whose
// background can be keyed out and faded into a solid colour, or into
// transparency made of solid dots (for the web or for DTF printing).
//
// It has no file or flag handling, so the same code can back the CLI
// (cmd/dithr) and, later, a WebAssembly build.
package dithr

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"

	"github.com/makeworld-the-better-one/dither/v2"
	xdraw "golang.org/x/image/draw"
)

// Options controls the whole pipeline. Start from DefaultOptions().
type Options struct {
	// Size
	Width int // output width in px; 0 = keep source width
	Pixel int // size of one dither pixel in output px (1 = fine, 3+ = chunky 8-bit)

	// Smooth is the Kuwahara radius in working px: flattens photo detail into
	// painted-looking areas (1-3 is typical, 0 = off).
	Smooth int

	// Tone adjustments, applied before keying (so they also move what gets cut out)
	LevelsLo, LevelsHi float64 // input black / white points, 0..1
	Gamma              float64 // >1 brightens midtones, <1 darkens
	Contrast           float64 // -1..1, 0 = unchanged
	Brightness         float64 // -1..1, 0 = unchanged
	Saturation         float64 // 0 = grayscale, 1 = unchanged, >1 = more vivid

	// Background cutout
	Key        string      // none | light | dark | color
	KeyColor   color.RGBA  // colour to remove when Key == "color"
	Cutoff     float64     // lightness (light/dark) or colour distance (color) where the background starts; <0 = auto
	Feather    float64     // width of the soft transition after the cutoff
	MaskBlur   int         // spatial blur of the key mask in working px; softens ragged edges
	Fade       []string    // extra gradient fades: top, bottom, left, right, radial
	FadeSize   float64     // how far the fades reach into the image, 0..1
	Background *color.RGBA // colour the background becomes; nil = black for dark key, otherwise white

	// Transparent output (see alpha.go). Instead of blending into Background,
	// the mask becomes alpha, and every pixel is fully opaque or fully transparent.
	Transparent bool
	Alpha       string  // threshold | dither | halftone
	LPI         float64 // halftone: dot lines per inch
	DPI         float64 // halftone: print resolution (output px per inch)
	Angle       float64 // halftone: screen angle in degrees
	Dot         string  // halftone: round | square | diamond | line
	MinDot      float64 // halftone: coverage below this is dropped, above 1-MinDot is solid (0..0.5)

	// Colours
	FullColor bool         // keep the original colours: no palette, no dithering
	Palette   []color.RGBA // fixed palette; nil = extract from the image
	Colors    int          // colours to extract (total, including the background colour)

	// Dithering
	Dither   string  // see DitherNames()
	Strength float64 // 0 or 1 = full; lower = flatter, more posterized
}

// DefaultOptions returns sensible defaults: no cutout, 16 extracted colours, Bayer 8×8.
func DefaultOptions() Options {
	return Options{
		Pixel:      1,
		LevelsHi:   1,
		Gamma:      1,
		Saturation: 1,
		Key:        "none",
		KeyColor:   color.RGBA{0, 0, 255, 255},
		Cutoff:     -1,
		Feather:    0.1,
		FadeSize:   0.3,
		Colors:     16,
		Dither:     "bayer8",
		Alpha:      "dither",
		LPI:        35,
		DPI:        300,
		Angle:      22.5,
		Dot:        "round",
	}
}

var diffusers = map[string]dither.ErrorDiffusionMatrix{
	"floyd":      dither.FloydSteinberg,
	"atkinson":   dither.Atkinson,
	"jjn":        dither.JarvisJudiceNinke,
	"stucki":     dither.Stucki,
	"burkes":     dither.Burkes,
	"sierra":     dither.Sierra,
	"sierra2":    dither.TwoRowSierra,
	"sierralite": dither.SierraLite,
	"pigeon":     dither.StevenPigeon,
}

var bayerSizes = map[string]uint{"bayer2": 2, "bayer4": 4, "bayer8": 8}

var orderedMatrices = map[string]dither.OrderedDitherMatrix{
	"dots4":  dither.ClusteredDot4x4,
	"dots8":  dither.ClusteredDot8x8,
	"diag8":  dither.ClusteredDotDiagonal8x8,
	"spiral": dither.ClusteredDotSpiral5x5,
	"hlines": dither.ClusteredDotHorizontalLine,
	"vlines": dither.ClusteredDotVerticalLine,
}

// DitherNames lists every supported dither mode.
func DitherNames() []string {
	names := []string{"none"}
	for k := range bayerSizes {
		names = append(names, k)
	}
	for k := range orderedMatrices {
		names = append(names, k)
	}
	for k := range diffusers {
		names = append(names, k)
	}
	sort.Strings(names[1:])
	return names
}

func (o *Options) normalize() error {
	if o.Pixel < 1 {
		o.Pixel = 1
	}
	if o.Gamma <= 0 {
		return fmt.Errorf("gamma must be > 0")
	}
	if o.LevelsHi <= o.LevelsLo {
		return fmt.Errorf("levels: white point must be above black point")
	}
	o.Key = strings.ToLower(o.Key)
	if o.Key == "" {
		o.Key = "none"
	}
	switch o.Key {
	case "none", "light", "dark", "color":
	default:
		return fmt.Errorf("unknown key %q (none, light, dark, color)", o.Key)
	}
	if o.Cutoff < 0 {
		o.Cutoff = map[string]float64{"light": 0.9, "dark": 0.2, "color": 0.15, "none": 0}[o.Key]
	}
	if o.Feather < 0 {
		o.Feather = 0
	}
	if o.Background == nil {
		bg := color.RGBA{255, 255, 255, 255}
		if o.Key == "dark" {
			bg = color.RGBA{0, 0, 0, 255}
		}
		o.Background = &bg
	}
	fades := make([]string, len(o.Fade)) // copy: never modify the caller's slice
	for i, f := range o.Fade {
		f = strings.ToLower(strings.TrimSpace(f))
		switch f {
		case "top", "bottom", "left", "right", "radial":
			fades[i] = f
		default:
			return fmt.Errorf("unknown fade %q (top, bottom, left, right, radial)", f)
		}
	}
	o.Fade = fades
	if len(o.Fade) > 0 && (o.FadeSize <= 0 || o.FadeSize > 1) {
		return fmt.Errorf("fade size must be above 0 and at most 1")
	}
	o.Alpha = strings.ToLower(o.Alpha)
	o.Dot = strings.ToLower(o.Dot)
	switch o.Alpha {
	case "":
		o.Alpha = "dither"
	case "threshold", "dither", "halftone":
	default:
		return fmt.Errorf("unknown alpha mode %q (threshold, dither, halftone)", o.Alpha)
	}
	switch o.Dot {
	case "":
		o.Dot = "round"
	case "round", "square", "diamond", "line":
	default:
		return fmt.Errorf("unknown dot %q (round, square, diamond, line)", o.Dot)
	}
	if o.Transparent && o.Alpha == "halftone" {
		if o.LPI <= 0 || o.DPI <= 0 {
			return fmt.Errorf("halftone needs LPI and DPI > 0")
		}
		if o.DPI/o.LPI < 3 {
			return fmt.Errorf("halftone cells are %.1f px; raise DPI or lower LPI (need at least 3 px)", o.DPI/o.LPI)
		}
	}
	if o.MinDot < 0 || o.MinDot >= 0.5 {
		return fmt.Errorf("min dot must be in 0..0.5")
	}
	maxColors := 256
	if o.Transparent {
		maxColors = 255 // one palette index is needed for transparency
	}
	// Transparency comes only from the keep mask, never from a palette entry.
	// (Rejected rather than forced opaque: color.RGBA is premultiplied, so
	// any guess at the intended colour could be wrong.)
	for _, c := range o.Palette {
		if c.A != 255 {
			return fmt.Errorf("palette colour %v is not opaque", c)
		}
	}
	if !o.FullColor {
		switch {
		case o.Palette != nil && len(o.Palette) == 0:
			return fmt.Errorf("palette is empty")
		case o.Palette != nil && len(o.Palette) > maxColors:
			return fmt.Errorf("palette has %d colours; the maximum is %d", len(o.Palette), maxColors)
		case o.Palette == nil && (o.Colors < 2 || o.Colors > maxColors):
			return fmt.Errorf("colors must be between 2 and %d", maxColors)
		}
	}
	o.Dither = strings.ToLower(o.Dither)
	if o.Dither == "" {
		o.Dither = "bayer8"
	}
	_, isDiff := diffusers[o.Dither]
	_, isBayer := bayerSizes[o.Dither]
	_, isOrd := orderedMatrices[o.Dither]
	if !isDiff && !isBayer && !isOrd && o.Dither != "none" {
		return fmt.Errorf("unknown dither %q (try: %s)", o.Dither, strings.Join(DitherNames(), ", "))
	}
	return nil
}

// buffer is the working image: float sRGB pixels plus a keep-mask.
type buffer struct {
	w, h  int
	px    []rgb
	alpha []float64 // 1 = keep, 0 = background
}

// Process runs the full pipeline. It returns an *image.Paletted (small
// indexed PNG/GIF), or an *image.NRGBA when FullColor is set.
func Process(src image.Image, o Options) (image.Image, error) {
	if err := o.normalize(); err != nil {
		return nil, err
	}

	b := load(src, o)
	smooth(b, o.Smooth)
	adjust(b, o)
	masked := buildMask(b, o)
	solidBg := masked && !o.Transparent // background becomes a colour
	bg := *o.Background
	if solidBg {
		bgc := fromRGBA(bg)
		for i, a := range b.alpha {
			p := b.px[i]
			b.px[i] = rgb{bgc.R + (p.R-bgc.R)*a, bgc.G + (p.G-bgc.G)*a, bgc.B + (p.B-bgc.B)*a}
		}
	}

	var keep []bool
	ow, oh := b.w*o.Pixel, b.h*o.Pixel
	if o.Transparent {
		keep, ow, oh = keepMask(b, o)
	}
	if o.FullColor {
		return fullColor(b, keep, o.Pixel, ow, oh), nil
	}

	pal := o.Palette
	if pal == nil {
		n := o.Colors
		if solidBg {
			n-- // keep one slot for the background
		}
		pal = extractPalette(b, visibleWeight(b, keep, o.Pixel), max(n, 1))
	}
	if solidBg {
		pal = withColor(pal, bg)
	}
	if len(pal) > 256 { // fixed palette plus the added background colour
		return nil, fmt.Errorf("palette has %d colours; the maximum is 256", len(pal))
	}

	img := ditherBuffer(b, pal, o, keep)
	if o.Transparent {
		return transparentPaletted(img, keep, o.Pixel, ow, oh), nil
	}
	if solidBg {
		// Ordered dithering can sprinkle dots into flat areas; keep the fully
		// cut-out background perfectly clean.
		bgIdx := uint8(img.Palette.Index(bg))
		for i, a := range b.alpha {
			if a < 0.001 {
				img.Pix[i] = bgIdx
			}
		}
	}
	if o.Pixel > 1 {
		img = upscale(img, o.Pixel)
	}
	return img, nil
}

// load resizes the source to the working size (output width / pixel size).
func load(src image.Image, o Options) *buffer {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	outW := o.Width
	if outW <= 0 {
		outW = sw
	}
	ww := max(int(math.Round(float64(outW)/float64(o.Pixel))), 1)
	wh := max(int(math.Round(float64(sh)*float64(ww)/float64(sw))), 1)

	dst := image.NewNRGBA(image.Rect(0, 0, ww, wh))
	if ww == sw && wh == sh {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, xdraw.Src, nil)
	}

	b := &buffer{w: ww, h: wh, px: make([]rgb, ww*wh), alpha: make([]float64, ww*wh)}
	for i := range b.px {
		p := dst.Pix[i*4 : i*4+4]
		b.px[i] = rgb{float64(p[0]) / 255, float64(p[1]) / 255, float64(p[2]) / 255}
		b.alpha[i] = float64(p[3]) / 255
	}
	return b
}

func adjust(b *buffer, o Options) {
	tone := o.LevelsLo != 0 || o.LevelsHi != 1 || o.Gamma != 1 || o.Contrast != 0 || o.Brightness != 0
	f := func(v float64) float64 {
		v = clamp01((v - o.LevelsLo) / (o.LevelsHi - o.LevelsLo))
		if o.Gamma != 1 {
			v = math.Pow(v, 1/o.Gamma)
		}
		v = (v-0.5)*(1+o.Contrast) + 0.5 + o.Brightness
		return clamp01(v)
	}
	for i, p := range b.px {
		if tone {
			p = rgb{f(p.R), f(p.G), f(p.B)}
		}
		if o.Saturation != 1 {
			c := toOklab(p)
			c.A *= o.Saturation
			c.B *= o.Saturation
			p = fromOklab(c)
		}
		b.px[i] = p
	}
}

// keep is the output-res transparency mask (nil when not transparent).
func ditherBuffer(b *buffer, pal []color.RGBA, o Options, keep []bool) *image.Paletted {
	m, isDiffuser := diffusers[o.Dither]
	if !isDiffuser {
		// Ordered / none: our palette-aware ditherer (see ordered.go).
		s := o.Strength
		if s == 0 {
			s = 1
		}
		return orderedDither(b, pal, orderedMap(o.Dither), s, o.Transparent && o.Alpha == "dither")
	}

	// The library diffuses error through every pixel, including ones that end
	// up transparent. Give every hidden pixel one fixed palette colour so its
	// (invisible) original colour can't leak into visible neighbours.
	px := b.px
	if keep != nil {
		px = append([]rgb{}, b.px...)
		hidden := fromRGBA(pal[0])
		ow := b.w * o.Pixel
		for wy := 0; wy < b.h; wy++ {
			for wx := 0; wx < b.w; wx++ {
				if !anyKept(keep, ow, wx*o.Pixel, wy*o.Pixel, o.Pixel) {
					px[wy*b.w+wx] = hidden
				}
			}
		}
	}

	// Error diffusion: the dither library handles this well.
	img := image.NewRGBA(image.Rect(0, 0, b.w, b.h))
	for i, p := range px {
		c := toRGBA(p)
		copy(img.Pix[i*4:i*4+4], []uint8{c.R, c.G, c.B, 255})
	}
	cp := make([]color.Color, len(pal))
	for i, c := range pal {
		cp[i] = c
	}
	d := dither.NewDitherer(cp)
	if o.Strength != 0 && o.Strength != 1 {
		m = dither.ErrorDiffusionStrength(m, float32(o.Strength))
	}
	d.Matrix = m
	d.Serpentine = true
	return d.DitherPaletted(img)
}

func upscale(src *image.Paletted, n int) *image.Paletted {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewPaletted(image.Rect(0, 0, w*n, h*n), src.Palette)
	for y := 0; y < h*n; y++ {
		srow := src.Pix[(y/n)*src.Stride:]
		drow := dst.Pix[y*dst.Stride:]
		for x := 0; x < w*n; x++ {
			drow[x] = srow[x/n]
		}
	}
	return dst
}

// anyKept reports whether any output pixel of the n×n block at (x0, y0) is kept.
func anyKept(keep []bool, ow, x0, y0, n int) bool {
	for y := y0; y < y0+n; y++ {
		for x := x0; x < x0+n; x++ {
			if keep[y*ow+x] {
				return true
			}
		}
	}
	return false
}

// visibleWeight is how much of each working pixel ends up visible: its alpha,
// or with transparent output the kept share of its output block (exact for
// threshold, dither and halftone alike).
func visibleWeight(b *buffer, keep []bool, n int) []float64 {
	if keep == nil {
		return b.alpha
	}
	w := make([]float64, len(b.alpha))
	ow := b.w * n
	for wy := 0; wy < b.h; wy++ {
		for wx := 0; wx < b.w; wx++ {
			kept := 0
			for y := wy * n; y < (wy+1)*n; y++ {
				for x := wx * n; x < (wx+1)*n; x++ {
					if keep[y*ow+x] {
						kept++
					}
				}
			}
			w[wy*b.w+wx] = float64(kept) / float64(n*n)
		}
	}
	return w
}
