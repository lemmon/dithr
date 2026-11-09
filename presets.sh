#!/usr/bin/env bash
# Favourite dithr presets.
#   ./presets.sh <preset> input.jpg [output.png] [extra dithr flags...]
# Extra flags override the preset's, e.g.:
#   ./presets.sh retro8 photo.jpg out.png --colors 6 --pixel 3
# Paths are relative to where you run it from, not to this script.
set -euo pipefail
dir=$(cd "$(dirname "$0")" && pwd)

usage() {
  echo "usage: $0 <preset> input [output.png] [extra flags...]"
  echo "presets: $(grep -oE '^  [a-z0-9-]+\)' "$0" | tr -d ' )' | tr '\n' ' ')"
  exit 1
}
[[ $# -ge 2 ]] || usage
preset=$1; in=$2; shift 2
base=$(basename "$in")
out="$(dirname "$in")/${base%.*}-$preset.png"
if [[ $# -gt 0 && $1 != -* ]]; then out=$1; shift; fi

case $preset in
  # painted retro-game look: photo detail flattened, Bayer only in gradients
  retro16)    flags=(--colors 16 --pixel 2 --dither bayer4 --smooth 2 --saturation 1.3) ;;
  retro8)     flags=(--colors 8 --pixel 2 --dither bayer4 --smooth 3 --saturation 1.3 --contrast 0.1) ;;
  # like the inspiration: bright sky / mist dissolves into white
  fog)        flags=(--colors 16 --pixel 2 --dither bayer4 --smooth 2 --saturation 1.3 --key light --cutoff 0.85 --feather 0.2) ;;
  # fixed classic palettes
  db16)       flags=(--palette db16 --pixel 2 --dither bayer4 --smooth 2) ;;
  pico8)      flags=(--palette pico8 --pixel 3 --dither floyd --smooth 1 --saturation 1.3) ;;
  gameboy)    flags=(--palette gameboy --pixel 3 --dither bayer4 --smooth 1 --contrast 0.25) ;;
  1bit)       flags=(--palette bw --pixel 2 --dither atkinson --smooth 1 --gamma 1.3) ;;
  win16)      flags=(--palette win16 --pixel 2 --dither bayer4 --smooth 2 --saturation 1.2) ;;
  synthwave)  flags=(--palette synthwave --pixel 2 --dither bayer4 --smooth 2 --contrast 0.15) ;;
  # monochrome: CRT monitors and print-like tints
  amber)      flags=(--palette amber --pixel 2 --dither bayer4 --smooth 1 --contrast 0.1) ;;
  phosphor)   flags=(--palette phosphor --pixel 2 --dither bayer4 --smooth 1 --contrast 0.1) ;;
  sepia)      flags=(--palette sepia --pixel 2 --dither bayer8 --smooth 2) ;;
  cyanotype)  flags=(--palette cyanotype --pixel 2 --dither bayer8 --smooth 2 --contrast 0.1) ;;
  # fog into the palette's paper colour instead of pure white
  sepia-fog)  flags=(--palette sepia --bg "#f4e8cc" --pixel 2 --dither bayer8 --smooth 2 --key light --cutoff 0.85 --feather 0.2) ;;
  # DTF print files: 11in wide at 300 dpi, transparent, every pixel solid or clear.
  # Fades/cutouts become halftone dots so the shirt shows through (no white-ink halos).
  dtf)                flags=(--palette full --bg transparent --alpha halftone --lpi 35 --angle 22.5 --min-dot 0.1 --print-width 11in) ;;
  dtf-vignette)       flags=(--palette full --bg transparent --alpha halftone --lpi 35 --angle 22.5 --min-dot 0.1 --print-width 11in --fade radial --fade-size 0.5) ;;
  dtf-knockout-dark)  flags=(--palette full --bg transparent --alpha halftone --lpi 35 --angle 22.5 --min-dot 0.1 --print-width 11in --key dark --cutoff 0.15 --feather 0.25) ;;
  dtf-knockout-light) flags=(--palette full --bg transparent --alpha halftone --lpi 35 --angle 22.5 --min-dot 0.1 --print-width 11in --key light --cutoff 0.88 --feather 0.25) ;;
  # retro pixels on a shirt: 6px pixels ≈ 0.5 mm at 300 dpi, pixel-dissolve transparency
  dtf-retro)          flags=(--colors 16 --pixel 6 --dither bayer4 --smooth 2 --saturation 1.3 --bg transparent --alpha dither --print-width 11in) ;;
  *) usage ;;
esac

(cd "$dir" && go build -o bin/dithr ./cmd/dithr)   # fast when nothing changed
"$dir/bin/dithr" "$in" -o "$out" "${flags[@]}" "$@"
