# dithr

A command-line tool that turns photos into dithered, palette-limited graphics with a retro
computer-game look, for websites and for print. It can cut the background out and fade it into a
solid colour, or into transparency made of solid dots: halftone dots for DTF shirt prints, or a
pixel dissolve for the web.

## Install

Needs Go 1.26 or newer.

```sh
go install github.com/lemmon/dithr/cmd/dithr@latest
dithr photo.jpg -o out.png --colors 8 --pixel 2 --dither bayer4 --smooth 2
dithr -h                                   # all flags
```

The presets and the demo are shell scripts in this repository, so clone it to use them:

```sh
git clone https://github.com/lemmon/dithr && cd dithr
./presets.sh retro16 photo.jpg out.png     # builds bin/dithr on first use
./demo.sh                                  # renders every image in samples/ through every preset
```

`samples/` is empty in the repository: add a few of your own photos there before running the demo.

## Pipeline

1. **Resize** to `--width / --pixel` working pixels.
   **Smooth** with `--smooth N` (Kuwahara filter, 1–3), which flattens photo detail into painted-looking areas.
   This is the main ingredient of the retro-game look: the dither then shows up in gradients instead of as noise.
2. **Adjust tone**: `--levels --gamma --contrast --brightness --saturation`.
   These run _before_ keying, so raising contrast also pushes more of the background past the cutoff.
3. **Mask** the background (1 = keep, 0 = background colour):
   - `--key light|dark --cutoff L` cuts by perceived lightness (0 black … 1 white).
   - `--key color --key-color "#4a90d9" --cutoff 0.2` cuts by colour distance (0.1 tight … 0.4 loose).
   - `--feather` sets the width of the soft ramp after the cutoff, and `--mask-blur N` softens ragged edges.
   - `--fade top,bottom,left,right,radial --fade-size 0.3` adds gradient fades.
   - Transparent PNGs work as input: transparency becomes background.
4. **Blend** toward `--bg` (auto: black for `--key dark`, otherwise white).
5. **Palette**: `--palette original --colors N` extracts N colours (background included).
   Other options are a built-in (`bw`, `gray`, `gameboy`, `cga`, `ega`, `c64`, `db16`, `pico8`, `win16`, `synthwave`, `amber`, `phosphor`, `sepia`, `cyanotype`), a hex list like `"#000,#c8643c,#fff"`, or a Lospec `.hex` file.
   `--print-palette` prints the palette used, so you can lock it in for the next image.
6. **Dither**:
   - Ordered: `bayer2/4/8`, plus clustered-dot `dots4 dots8 diag8 spiral hlines vlines`.
   - Error diffusion: `floyd atkinson jjn stucki burkes sierra sierra2 sierralite pigeon`.
   - `none` maps each pixel to the nearest colour with no dithering.
   - `--strength` below 1 flattens the result toward posterized.
7. **Upscale** by `--pixel` (nearest neighbour) and write an indexed PNG (or GIF).

## Presets (`presets.sh`)

| Preset                              | Look                                                                           |
| ----------------------------------- | ------------------------------------------------------------------------------ |
| `retro16` / `retro8`                | painted retro-game background, colours extracted from the image                |
| `fog`                               | like retro16, plus bright sky and mist dissolving into white                   |
| `db16`, `pico8`, `gameboy`, `win16` | fixed classic palettes (win16 = Windows 3.1 / VGA)                             |
| `synthwave`                         | purple / pink / yellow 80s palette                                             |
| `amber`, `phosphor`                 | monochrome CRT monitors                                                        |
| `sepia`, `cyanotype`                | print-like tints; sepia reads as an old adventure game                         |
| `sepia-fog`                         | sepia, with bright areas dissolving into the palette's paper colour            |
| `1bit`                              | black & white Atkinson, early-Mac style; add `--gamma 1.8` for dark landscapes |

Extra flags after the output path override the preset's.

## DTF print files

DTF printers lay solid white ink under any pixel that isn't 100% transparent, so soft fades print as
chalky halos. With `--bg transparent` the cutout/fade mask becomes the alpha channel instead of a
background colour, and every pixel is either fully opaque or fully transparent:

- `--alpha halftone`: round dots on a rotated print screen; the shirt shows through between them.
  `--lpi 35 --angle 22.5 --dot round|square|diamond|line`, and `--min-dot 0.1` drops dots too small to transfer.
- `--alpha dither`: pixel-level retro dissolve (also great for transparent web images).
- `--alpha threshold`: plain hard cut.

Sizing: `--print-width 11in` (or `28cm`) together with `--dpi 300` sets the pixel width, and the DPI is written into the PNG.
Use `--palette full` to keep the photo's colours untouched (no palette, no dithering) and halftone only the fades.

```sh
./presets.sh dtf-knockout-dark photo.jpg tee.png           # drop the dark background for dark shirts
./presets.sh dtf-vignette photo.jpg tee.png                # oval fade-out
./presets.sh dtf photo.jpg tee.png --key light --cutoff 0.9 --print-width 12in
```

Check dot size, LPI and minimum dot with your printer/RIP before a big run, and use a high-resolution source
(the halftone is crisp at any size, but the photo inside it isn't).

## Layout

- `*.go` (package `dithr`): the image pipeline. No file or flag handling, so it can be compiled to Wasm later.
- `cmd/dithr/`: the CLI wrapper.
- `alpha.go`: transparent output (threshold, dither, halftone).
- `ordered.go`: palette-aware ordered dithering (Knoll pattern dithering).
- `presets.sh`: named presets; `demo.sh`: renders `samples/` into a comparison page.

## Credits

dithr stands on other people's work:

- [makeworld-the-better-one/dither](https://github.com/makeworld-the-better-one/dither) (MPL-2.0)
  does the error-diffusion dithering. Its author also makes [didder](https://github.com/makew0rld/didder),
  a full-featured dithering CLI; use it if you don't need the background cutout or halftone fades.
- [disintegration/imaging](https://github.com/disintegration/imaging) (MIT) reads EXIF orientation,
  and [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) (BSD) does resizing and WebP.
- Ordered dithering for arbitrary palettes uses Thomas Knoll's pattern dithering, as explained in
  [Joel Yliluoma's dithering article](https://bisqwit.iki.fi/story/howto/dither/jy/).
- Colour distances use [OKLab](https://bottosson.github.io/posts/oklab/) by Björn Ottosson.
- Built-in palettes include DawnBringer's DB16 and the PICO-8 palette by Lexaloffle.
  More palettes are on [Lospec](https://lospec.com/palette-list); their `.hex` files work with `--palette`.
- For colour (CMYK) halftone as a look rather than for DTF fades, see
  [halftone-converter](https://github.com/curegit/halftone-converter) or [G'MIC](https://gmic.eu/).

## License

MIT, see [LICENSE](LICENSE).
