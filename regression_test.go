package dithr

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func fill(w, h int, f func(x, y int) color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, f(x, y))
		}
	}
	return img
}

// A 50%-opaque light grey with a bw palette must keep a mix of black and
// white, not only black, when alpha and colour are both ordered-dithered.
func TestAlphaDitherKeepsColourMix(t *testing.T) {
	src := fill(64, 64, func(x, y int) color.NRGBA { return color.NRGBA{0xbc, 0xbc, 0xbc, 128} })
	o := DefaultOptions()
	o.Transparent, o.Alpha, o.Dither = true, "dither", "bayer4"
	o.Palette = []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	white, kept := 0, 0
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				kept++
				if r > 0x8000 {
					white++
				}
			}
		}
	}
	if f := float64(white) / float64(kept); f < 0.3 || f > 0.7 {
		t.Errorf("white share of kept pixels = %.2f (kept %d); want a real mix", f, kept)
	}
}

// Faint (30%) blue must still get a palette colour in transparent mode.
func TestPaletteIncludesFaintPixels(t *testing.T) {
	src := fill(64, 64, func(x, y int) color.NRGBA {
		if x < 32 {
			return color.NRGBA{255, 0, 0, 255}
		}
		return color.NRGBA{0, 0, 255, 77}
	})
	o := DefaultOptions()
	o.Transparent, o.Alpha, o.Dither, o.Colors = true, "dither", "none", 2
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 64; y++ {
		for x := 32; x < 64; x++ {
			r, _, bl, a := img.At(x, y).RGBA()
			if a > 0 && r > bl {
				t.Fatalf("visible pixel %d,%d in the blue half is red", x, y)
			}
		}
	}
}

// Hidden pixels' RGB must not change visible output with error diffusion,
// whether they are fully transparent or only hidden by the keep mask (30% < 50% threshold).
func TestHiddenPixelsDontLeakError(t *testing.T) {
	for _, a := range []uint8{0, 77} {
		testHiddenPixels(t, a)
	}
}

func testHiddenPixels(t *testing.T, alpha uint8) {
	render := func(hidden color.NRGBA) []byte {
		src := fill(32, 32, func(x, y int) color.NRGBA {
			if x < 16 {
				return color.NRGBA{120, 120, 120, 255}
			}
			return hidden
		})
		o := DefaultOptions()
		o.Transparent, o.Alpha, o.Dither = true, "threshold", "floyd"
		o.Palette = []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}
		img, err := Process(src, o)
		if err != nil {
			t.Fatal(err)
		}
		return img.(*image.Paletted).Pix
	}
	if !bytes.Equal(render(color.NRGBA{0, 0, 0, alpha}), render(color.NRGBA{255, 255, 255, alpha})) {
		t.Errorf("alpha %d: output depends on the RGB of hidden pixels", alpha)
	}
}

// Smoothing must not pull invisible colours into visible pixels.
func TestSmoothIgnoresTransparentColours(t *testing.T) {
	render := func(bg color.NRGBA) color.Color {
		src := fill(9, 9, func(x, y int) color.NRGBA {
			if x == 4 && y == 4 {
				return color.NRGBA{255, 0, 0, 255}
			}
			return bg
		})
		o := DefaultOptions()
		o.Transparent, o.Alpha, o.FullColor, o.Smooth = true, "threshold", true, 1
		img, err := Process(src, o)
		if err != nil {
			t.Fatal(err)
		}
		return img.At(4, 4)
	}
	a, b := render(color.NRGBA{0, 0, 0, 0}), render(color.NRGBA{255, 255, 255, 0})
	if a != b || a != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("centre = %v / %v, want pure red in both", a, b)
	}
}

// bayer8 must actually differ from bayer4.
func TestLargerBayerDiffers(t *testing.T) {
	src := fill(128, 32, func(x, y int) color.NRGBA { v := uint8(x * 2); return color.NRGBA{v, v, v, 255} })
	out := map[string][]byte{}
	for _, d := range []string{"bayer4", "bayer8"} {
		o := DefaultOptions()
		o.Dither = d
		o.Palette = []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}
		img, err := Process(src, o)
		if err != nil {
			t.Fatal(err)
		}
		out[d] = img.(*image.Paletted).Pix
	}
	if bytes.Equal(out["bayer4"], out["bayer8"]) {
		t.Error("larger Bayer matrices produce the same output as smaller ones")
	}
}

// Option validation and caller safety.
func TestOptionValidation(t *testing.T) {
	src := fill(8, 8, func(x, y int) color.NRGBA { return color.NRGBA{100, 100, 100, 255} })

	o := DefaultOptions()
	o.Palette = []color.RGBA{}
	if _, err := Process(src, o); err == nil {
		t.Error("empty palette should be an error, not a panic")
	}

	o = DefaultOptions()
	o.Colors = 300
	if _, err := Process(src, o); err == nil {
		t.Error("colors above 256 should be rejected up front")
	}

	o = DefaultOptions()
	o.FadeSize = 0 // no fade requested, so this must not matter
	if _, err := Process(src, o); err != nil {
		t.Errorf("unused fade size rejected: %v", err)
	}

	o = DefaultOptions()
	o.Transparent, o.Alpha, o.Dot = true, "Halftone", "Round"
	if _, err := Process(src, o); err != nil {
		t.Errorf("mixed-case alpha/dot rejected: %v", err)
	}

	o = DefaultOptions()
	fades := []string{" TOP "}
	o.Fade = fades
	if _, err := Process(src, o); err != nil {
		t.Fatal(err)
	}
	if fades[0] != " TOP " {
		t.Errorf("Process modified the caller's Fade slice: %q", fades[0])
	}
}

// With threshold transparency, pixels below 50% alpha are cut away, so they
// must not take palette slots from the visible ones.
func TestPaletteIgnoresDiscardedPixels(t *testing.T) {
	src := fill(64, 64, func(x, y int) color.NRGBA {
		switch x {
		case 0:
			return color.NRGBA{255, 0, 0, 255}
		case 1:
			return color.NRGBA{0, 255, 0, 255}
		}
		return color.NRGBA{0, 0, 255, 77}
	})
	o := DefaultOptions()
	o.Transparent, o.Alpha, o.Dither, o.Colors = true, "threshold", "none", 2
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 10).RGBA()
	if !(r > g && r > b) {
		t.Errorf("red column came out %d,%d,%d", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(1, 10).RGBA()
	if !(g > r && g > b) {
		t.Errorf("green column came out %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

// Halftone with chunky pixels must not put dots over fully transparent
// working pixels: their RGB is invisible and arbitrary (here black).
func TestHalftoneChunkyPixelsShowNoHiddenColour(t *testing.T) {
	// 2×2 source, 8 px output with 4 px pixels: no resize, so the working
	// pixels are exactly opaque red and fully transparent black.
	src := fill(2, 2, func(x, y int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{255, 0, 0, 255}
		}
		return color.NRGBA{0, 0, 0, 0}
	})
	o := DefaultOptions()
	o.Transparent, o.Alpha, o.FullColor, o.Width, o.Pixel = true, "halftone", true, 8, 4
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if r, _, _, a := img.At(x, y).RGBA(); a > 0 && r < 0x8000 {
				t.Fatalf("pixel %d,%d shows a hidden pixel's colour", x, y)
			}
		}
	}
}

// A caller's palette with a semi-transparent colour must be rejected: it
// would otherwise produce semi-transparent output pixels.
func TestNonOpaquePaletteIsRejected(t *testing.T) {
	src := fill(32, 32, func(x, y int) color.NRGBA { return color.NRGBA{64, 0, 0, 255} })
	o := DefaultOptions()
	o.Transparent = true
	o.Palette = []color.RGBA{{64, 0, 0, 128}, {255, 255, 255, 255}}
	if _, err := Process(src, o); err == nil {
		t.Error("semi-transparent palette colour was accepted")
	}
}
