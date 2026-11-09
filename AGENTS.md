# Notes for AI agents working with dithr

**This is a public repository** (github.com/lemmon/dithr, MIT).

- Never commit secrets, personal paths (`/Users/...`), email addresses or private project details.
- Never commit images we don't own. `samples/`, `demo/` and reference images stay local
  (see `.gitignore`); tests build their test images in code.
- Write README, docs and commit messages for outside readers.
- dithr builds on other people's work (see README, "Credits"). Mention other tools respectfully,
  describe what each does well and how dithr differs, and credit ideas taken from them.

- The root package is the image pipeline: no file, flag or terminal handling, so it can be
  compiled to Wasm later. All of that lives in `cmd/dithr`.
- The CLI takes its defaults from `DefaultOptions()`. Don't restate default values in flags.
- In transparent mode every output pixel is fully opaque or fully transparent: DTF printers put
  white ink under any partial alpha. Keep the tests that check this.
- Colour and alpha dithering must use the same threshold map (`orderedMap`); `orderedDither`
  depends on it to keep the colour mix right in fades.
- Fix bugs by writing a failing test first. Before finishing, run `gofmt -l .`, `go vet ./...`
  and `go test ./...`.
- Prefer small fixes. If a fix grows into a rewrite (for example, our own error diffusion),
  stop and say so instead.
- Judge visual changes on 1:1 crops; dithering disappears when an image is scaled down.
  `./demo.sh` renders every image in `samples/` through every preset.
- Keep `README.md` and `presets.sh` up to date when flags or presets change.
