package dithr

import (
	"image"
	"image/color"
	"testing"
)

func TestParseHexColor(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#fff":    {255, 255, 255, 255},
		"00f":     {0, 0, 255, 255},
		"#1a2B3c": {0x1a, 0x2b, 0x3c, 255},
		"black":   {0, 0, 0, 255},
	} {
		got, err := ParseHexColor(in)
		if err != nil || got != want {
			t.Errorf("ParseHexColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseHexColor("#12"); err == nil {
		t.Error("expected error for #12")
	}
}

func TestParsePalette(t *testing.T) {
	if p, _ := ParsePalette("original", 8); p != nil {
		t.Error("original should mean extract (nil)")
	}
	if p, _ := ParsePalette("gray", 5); len(p) != 5 || p[4] != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("gray 5 = %v", p)
	}
	if p, _ := ParsePalette("ff0000\n00ff00\n", 0); len(p) != 2 {
		t.Errorf("hex file contents = %v", p)
	}
}

// A dark left half and a bright right gradient: the dark key must turn the
// left half into exactly the background colour, whatever the dither mode.
func TestDarkKeyGivesCleanBackground(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(10)
			if x >= 32 {
				v = uint8(120 + x*2)
			}
			src.Set(x, y, color.RGBA{v, v / 2, v / 3, 255})
		}
	}
	for _, d := range []string{"none", "bayer4", "dots8", "floyd", "atkinson"} {
		o := DefaultOptions()
		o.Key, o.Colors, o.Dither = "dark", 4, d
		out, err := Process(src, o)
		if err != nil {
			t.Fatal(err)
		}
		img := out.(*image.Paletted)
		bg := img.Palette.Index(color.RGBA{0, 0, 0, 255})
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				if int(img.ColorIndexAt(x, y)) != bg {
					t.Fatalf("%s: pixel %d,%d is not background", d, x, y)
				}
			}
		}
	}
}

func TestPixelUpscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	o := DefaultOptions()
	o.Pixel, o.Colors = 4, 2
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 52 { // 25×13 working px × 4
		t.Errorf("size = %v", img.Bounds())
	}
}

// opaqueFraction counts fully opaque pixels and fails on any partial alpha.
func opaqueFraction(t *testing.T, img image.Image) float64 {
	t.Helper()
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			switch a {
			case 0xffff:
				n++
			case 0:
			default:
				t.Fatalf("pixel %d,%d has partial alpha %d", x, y, a)
			}
		}
	}
	return float64(n) / float64(b.Dx()*b.Dy())
}

// A source that is uniformly 30% opaque must come out as ~30% solid pixels
// and 70% fully transparent ones, in every transparent mode and colour mode.
func TestTransparentModesAreBinaryAndKeepCoverage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 240, 240))
	for i := 0; i < len(src.Pix); i += 4 {
		copy(src.Pix[i:i+4], []uint8{200, 120, 60, 77}) // 77/255 ≈ 0.30
	}
	for _, mode := range []string{"dither", "halftone"} {
		for _, full := range []bool{false, true} {
			o := DefaultOptions()
			o.Transparent, o.Alpha, o.FullColor, o.Colors = true, mode, full, 4
			o.DPI, o.LPI = 300, 30 // 10 px cells
			img, err := Process(src, o)
			if err != nil {
				t.Fatal(err)
			}
			if f := opaqueFraction(t, img); f < 0.27 || f > 0.33 {
				t.Errorf("%s full=%v: opaque fraction %.3f, want ≈0.30", mode, full, f)
			}
		}
	}
}

func TestHalftoneMinDot(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(src.Pix); i += 4 {
		copy(src.Pix[i:i+4], []uint8{0, 0, 0, 20}) // ≈8% coverage
	}
	o := DefaultOptions()
	o.Transparent, o.Alpha, o.FullColor, o.MinDot = true, "halftone", true, 0.1
	img, err := Process(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if f := opaqueFraction(t, img); f != 0 {
		t.Errorf("dots under the minimum should be dropped, got %.3f opaque", f)
	}
}
