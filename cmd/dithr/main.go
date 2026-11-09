// Command dithr converts an image into a dithered, palette-limited graphic
// with an optional background cutout. Run `dithr -h` for all flags.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lemmon/dithr"

	"github.com/disintegration/imaging"

	_ "golang.org/x/image/webp"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dithr:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	o := dithr.DefaultOptions()
	fs := flag.NewFlagSet("dithr", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: dithr [flags] input.(jpg|png|webp|gif) [-o output.png]\n\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\nPalettes: %s, a hex list like \"#000,#e0c9a6,#fff\", or a .hex file\n", strings.Join(dithr.PaletteNames(), ", "))
		fmt.Fprintf(fs.Output(), "Dithers:  %s\n", strings.Join(dithr.DitherNames(), ", "))
	}

	out := fs.String("o", "", "output file, .png or .gif (default: <input>-dithr.png)")
	fs.IntVar(&o.Width, "width", o.Width, "output width in px (0 = source width)")
	fs.IntVar(&o.Pixel, "pixel", o.Pixel, "size of one dither pixel in px; 2-4 gives a chunky 8-bit look")

	fs.IntVar(&o.Smooth, "smooth", o.Smooth, "painterly smoothing radius in working px (Kuwahara); 1-3 flattens photo detail")

	levels := fs.String("levels", fmt.Sprintf("%g,%g", o.LevelsLo, o.LevelsHi), "input black,white points (0..1), e.g. 0.1,0.85")
	fs.Float64Var(&o.Gamma, "gamma", o.Gamma, "midtone gamma: >1 brightens, <1 darkens")
	fs.Float64Var(&o.Contrast, "contrast", o.Contrast, "contrast, -1..1")
	fs.Float64Var(&o.Brightness, "brightness", o.Brightness, "brightness, -1..1")
	fs.Float64Var(&o.Saturation, "saturation", o.Saturation, "saturation: 0 = gray, 1 = unchanged, >1 = vivid")

	fs.StringVar(&o.Key, "key", o.Key, "background cutout: none | light | dark | color")
	keyColor := fs.String("key-color", dithr.HexList([]color.RGBA{o.KeyColor}), "colour to cut out with --key color")
	fs.Float64Var(&o.Cutoff, "cutoff", o.Cutoff, "where the background starts: lightness 0..1 for light/dark keys,\ncolour distance for --key color (0.1 tight .. 0.4 loose); -1 = auto")
	fs.Float64Var(&o.Feather, "feather", o.Feather, "soft transition width after the cutoff (same units)")
	fs.IntVar(&o.MaskBlur, "mask-blur", o.MaskBlur, "blur the cutout edge by N working px")
	fade := fs.String("fade", "", "gradient fade(s) into the background: top,bottom,left,right,radial")
	fs.Float64Var(&o.FadeSize, "fade-size", o.FadeSize, "how far fades reach into the image, 0..1")
	bg := fs.String("bg", "auto", "background colour (auto = black for --key dark, else white),\nor \"transparent\": the cutout becomes solid dots in the alpha channel")

	alpha := fs.String("alpha", "", "with --bg transparent: dither (retro pixel dissolve, default) | halftone (print dots, for DTF) | threshold")
	fs.Float64Var(&o.LPI, "lpi", o.LPI, "halftone: dot lines per inch")
	fs.Float64Var(&o.DPI, "dpi", o.DPI, "print resolution; used for halftone dot size and --print-width, and written into the PNG")
	fs.Float64Var(&o.Angle, "angle", o.Angle, "halftone: screen angle in degrees")
	fs.StringVar(&o.Dot, "dot", o.Dot, "halftone: dot shape round | square | diamond | line")
	fs.Float64Var(&o.MinDot, "min-dot", o.MinDot, "halftone: drop dots below this coverage (and fill holes above 1-x), e.g. 0.1;\ntiny dots may not survive a DTF transfer")
	printWidth := fs.String("print-width", "", "print width like 11in or 28cm; sets --width from --dpi")

	palette := fs.String("palette", "original", "palette: original (extract from image), full (keep all colours, no dithering),\na built-in name, hex list or .hex file")
	fs.IntVar(&o.Colors, "colors", o.Colors, "number of colours for original/gray palettes, including the background")
	fs.StringVar(&o.Dither, "dither", o.Dither, "dither algorithm (see list below)")
	fs.Float64Var(&o.Strength, "strength", o.Strength, "dither strength 0..1 (0 = default 1); lower = flatter, more posterized")
	printPal := fs.Bool("print-palette", false, "print the palette used, as a reusable --palette value")

	// Allow flags before and after the input path.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one input file, got %d", len(pos))
	}
	in := pos[0]

	var err error
	if o.LevelsLo, o.LevelsHi, err = parsePair(*levels); err != nil {
		return fmt.Errorf("--levels: %w", err)
	}
	if o.KeyColor, err = dithr.ParseHexColor(*keyColor); err != nil {
		return fmt.Errorf("--key-color: %w", err)
	}
	if o.DPI <= 0 {
		return fmt.Errorf("--dpi must be above 0")
	}
	transparent := strings.EqualFold(*bg, "transparent")
	if transparent {
		o.Transparent = true
		if *alpha != "" {
			o.Alpha = *alpha
		}
	} else if *alpha != "" {
		// e.g. a dtf preset with --bg white added: the later choice wins
		fmt.Fprintln(os.Stderr, "dithr: note: --alpha is ignored without --bg transparent")
	}
	if *printWidth != "" {
		if o.Width != 0 {
			// e.g. a dtf preset with --width added: the explicit pixel width wins
			fmt.Fprintln(os.Stderr, "dithr: note: --width overrides --print-width")
		} else {
			inches, err := parseLength(*printWidth)
			if err != nil {
				return fmt.Errorf("--print-width: %w", err)
			}
			o.Width = int(math.Round(inches * o.DPI))
		}
	}
	if !strings.EqualFold(*bg, "auto") && !transparent {
		c, err := dithr.ParseHexColor(*bg)
		if err != nil {
			return fmt.Errorf("--bg: %w", err)
		}
		o.Background = &c
	}
	if *fade != "" {
		o.Fade = strings.Split(*fade, ",")
	}
	if strings.EqualFold(*palette, "full") {
		o.FullColor = true
	} else if o.Palette, err = resolvePalette(*palette, o.Colors); err != nil {
		return err
	}

	if *out == "" {
		*out = strings.TrimSuffix(in, filepath.Ext(in)) + "-dithr.png"
	}
	// Check the output type before the (possibly long) render.
	switch ext := strings.ToLower(filepath.Ext(*out)); {
	case ext != ".png" && ext != ".gif":
		return fmt.Errorf("output must be .png or .gif (lossy formats would smear the dither pattern)")
	case ext == ".gif" && o.FullColor:
		return fmt.Errorf("--palette full needs .png output (GIF is limited to 256 colours)")
	}

	src, err := readImage(in)
	if err != nil {
		return err
	}
	img, err := dithr.Process(src, o)
	if err != nil {
		return err
	}

	if err := writeImage(*out, img, o.DPI); err != nil {
		return err
	}
	b := img.Bounds()
	desc := "full colour"
	if p, ok := img.(*image.Paletted); ok {
		desc = fmt.Sprintf("%d colours", len(p.Palette))
	}
	if o.Transparent {
		desc += ", transparent " + o.Alpha
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%dx%d = %.2fx%.2f in at %g dpi, %s)\n", *out, b.Dx(), b.Dy(),
		float64(b.Dx())/o.DPI, float64(b.Dy())/o.DPI, o.DPI, desc)
	if p, ok := img.(*image.Paletted); ok && *printPal {
		var pal []color.RGBA
		for _, c := range p.Palette {
			if rc := color.RGBAModel.Convert(c).(color.RGBA); rc.A == 255 {
				pal = append(pal, rc)
			}
		}
		fmt.Println(dithr.HexList(pal))
	}
	return nil
}

// parseLength parses "11in", "28cm", "280mm" or a bare number (inches) into inches.
func parseLength(s string) (float64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	unit := 1.0
	for suffix, f := range map[string]float64{"in": 1, "cm": 1 / 2.54, "mm": 1 / 25.4} {
		if strings.HasSuffix(s, suffix) {
			s, unit = strings.TrimSuffix(s, suffix), f
			break
		}
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("want a length like 11in, 28cm or 280mm")
	}
	return v * unit, nil
}

func parsePair(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("want two numbers like 0.1,0.9")
	}
	a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("want two numbers like 0.1,0.9")
	}
	return a, b, nil
}

func resolvePalette(spec string, colors int) ([]color.RGBA, error) {
	if fi, err := os.Stat(spec); err == nil && !fi.IsDir() {
		data, err := os.ReadFile(spec)
		if err != nil {
			return nil, err
		}
		spec = string(data)
	}
	return dithr.ParsePalette(spec, colors)
}

// readImage decodes any registered format (JPEG, PNG, GIF, WebP) and applies
// the EXIF orientation, so phone photos aren't sideways.
func readImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := imaging.Decode(f, imaging.AutoOrientation(true))
	if err != nil {
		if strings.EqualFold(filepath.Ext(path), ".avif") || strings.EqualFold(filepath.Ext(path), ".heic") {
			return nil, fmt.Errorf("%s: format not supported; convert first: sips -s format png %q --out in.png", path, path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

func writeImage(path string, img image.Image, dpi float64) error {
	var buf bytes.Buffer
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif":
		p, ok := img.(*image.Paletted)
		if !ok {
			return fmt.Errorf("--palette full needs .png output (GIF is limited to 256 colours)")
		}
		if err := gif.Encode(&buf, p, &gif.Options{NumColors: len(p.Palette)}); err != nil {
			return err
		}
	case ".png":
		level := png.BestCompression // indexed images are small; squeeze them
		if _, ok := img.(*image.Paletted); !ok {
			level = png.DefaultCompression // full colour print files: best is very slow
		}
		if err := (&png.Encoder{CompressionLevel: level}).Encode(&buf, img); err != nil {
			return err
		}
		withDPI(&buf, dpi)
	default:
		return fmt.Errorf("output must be .png or .gif (lossy formats would smear the dither pattern)")
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// withDPI inserts a pHYs chunk after IHDR so print and RIP software know the
// physical size. (Go's PNG encoder doesn't write one.)
func withDPI(buf *bytes.Buffer, dpi float64) {
	if dpi <= 0 {
		return
	}
	ppm := uint32(math.Round(dpi / 0.0254)) // pixels per metre
	chunk := make([]byte, 0, 21)
	chunk = binary.BigEndian.AppendUint32(chunk, 9)
	chunk = append(chunk, "pHYs"...)
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = append(chunk, 1) // unit: metre
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))

	const ihdrEnd = 8 + 25 // signature + IHDR chunk
	data := buf.Bytes()
	out := append(append(append([]byte{}, data[:ihdrEnd]...), chunk...), data[ihdrEnd:]...)
	buf.Reset()
	buf.Write(out)
}
