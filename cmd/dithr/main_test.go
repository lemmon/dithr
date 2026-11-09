package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// A landscape-stored JPEG tagged "rotate 90° CW" (EXIF orientation 6), the way
// phones save portrait photos, must load as portrait.
func TestReadImageAppliesEXIFOrientation(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte{
		'M', 'M', 0, 42, 0, 0, 0, 8, // big-endian TIFF header, IFD0 at offset 8
		0, 1, // one entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, 6, 0, 0, // Orientation (SHORT) = 6
		0, 0, 0, 0, // no next IFD
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	app1 := append([]byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
	data := buf.Bytes()
	tagged := append(append(append([]byte{}, data[:2]...), app1...), data[2:]...)

	path := filepath.Join(t.TempDir(), "phone.jpg")
	if err := os.WriteFile(path, tagged, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := readImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Errorf("size = %dx%d, want 20x40 (rotated)", b.Dx(), b.Dy())
	}
}

func TestCLIOverridesAndValidation(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	f, _ := os.Create(in)
	jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 50, 50)), nil) // content type sniffed, ext irrelevant
	f.Close()
	out := filepath.Join(dir, "out.png")

	// dtf-style flags plus overrides, as presets.sh passes them
	if err := run([]string{in, "-o", out, "--bg", "transparent", "--alpha", "halftone", "--print-width", "2in", "--width", "100"}); err != nil {
		t.Errorf("--width should override --print-width: %v", err)
	}
	if err := run([]string{in, "-o", out, "--alpha", "halftone", "--bg", "White"}); err != nil {
		t.Errorf("--alpha without transparent should be ignored, and --bg case-insensitive: %v", err)
	}
	if err := run([]string{in, "-o", out, "--bg", "Transparent"}); err != nil {
		t.Errorf("--bg Transparent rejected: %v", err)
	}
	if err := run([]string{in, "-o", out, "--print-width", "2in", "--dpi", "0"}); err == nil {
		t.Error("--dpi 0 should be rejected")
	}
}
