#!/usr/bin/env bash
# Renders every image in samples/ through every preset into demo/,
# plus demo/index.html to compare them. Put your own photos in samples/.
#   ./demo.sh
set -euo pipefail
cd "$(dirname "$0")"

shopt -s nullglob nocaseglob
images=(samples/*.jpg samples/*.jpeg samples/*.png samples/*.webp)
if [[ ${#images[@]} -eq 0 ]]; then
  echo "No images in samples/. Add a few photos (jpg, png or webp) and run again." >&2
  exit 1
fi

go build -o bin/dithr ./cmd/dithr
rm -rf demo && mkdir -p demo/dtf

screen_presets=$(grep -oE '^  [a-z0-9-]+\)' presets.sh | tr -d ' )' | grep -v '^dtf')
html=""

for img in "${images[@]}"; do
  base=$(basename "${img%.*}")
  html+="<h2>$base</h2><main>"
  for p in $screen_presets; do
    ./presets.sh "$p" "$img" "demo/$base-$p.png" --width 1440
    html+="<figure><a href=\"$base-$p.png\"><img src=\"$base-$p.png\" loading=lazy></a><figcaption>$p</figcaption></figure>"
  done
  html+="</main>"
done

# DTF print files: two presets that work on any photo, shown on shirt colours
# with a 1:1 zoom on the faded edge, where the dots are.
html+="<h2>DTF print files</h2><p>Transparent PNGs at print size (11&nbsp;in, 300&nbsp;dpi), shown on shirt colours. The last box is a 1:1 zoom on the faded edge.</p>"
for img in "${images[@]}"; do
  for job in "dtf-vignette|" "dtf-retro|--fade radial --fade-size 0.5"; do
    IFS='|' read -r preset extra <<<"$job"
    name="$(basename "${img%.*}")-$preset"
    # shellcheck disable=SC2086
    ./presets.sh "$preset" "$img" "demo/dtf/$name.png" $extra
    html+="<h3>$name</h3><div class=dtf>"
    for shirt in "#111" "#1f2a44" "#8a8d8f" "#f2f0ea"; do
      html+="<a href=\"dtf/$name.png\" style=\"background:$shirt\"><img src=\"dtf/$name.png\" loading=lazy></a>"
    done
    html+="<div class=zoom style=\"background:#111 url('dtf/$name.png') no-repeat 6% 50%\"></div></div>"
  done
done

cat >demo/index.html <<HTML
<!doctype html><meta charset="utf-8"><title>dithr demo</title>
<style>
body{margin:0;padding:24px;background:#151515;color:#ddd;font:14px/1.4 system-ui}
main{display:grid;grid-template-columns:repeat(auto-fill,minmax(420px,1fr));gap:20px}
img{width:100%;display:block;image-rendering:pixelated}figcaption{padding:6px 0}
.dtf{display:grid;grid-template-columns:repeat(5,1fr);gap:12px;align-items:center}
.dtf a{display:block;padding:12px;border-radius:6px}.dtf img{image-rendering:auto}
.zoom{height:320px;border-radius:6px}
</style><h1>dithr demo</h1><p>Click an image to see it at full size (the dither only reads at 1:1).</p>$html
HTML
echo "open demo/index.html"
