package dithr

import (
	"image/color"
	"math"
)

// rgb is an sRGB colour with channels in 0..1.
type rgb struct{ R, G, B float64 }

// lab is an OKLab colour. L is perceptual lightness (0 = black, 1 = white).
// Distances in OKLab roughly match how different two colours look.
type lab struct{ L, A, B float64 }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// smoothstep returns 0 below e0, 1 above e1 and a smooth ramp in between.
// With e0 == e1 it acts as a hard step.
func smoothstep(e0, e1, x float64) float64 {
	if e1 <= e0 {
		if x < e0 {
			return 0
		}
		return 1
	}
	t := clamp01((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSrgb(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func toOklab(c rgb) lab {
	return oklabFromLinear(srgbToLinear(c.R), srgbToLinear(c.G), srgbToLinear(c.B))
}

func oklabFromLinear(r, g, b float64) lab {
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return lab{
		L: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		A: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		B: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

func fromOklab(c lab) rgb {
	l := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s := c.L - 0.0894841775*c.A - 1.2914855480*c.B
	l, m, s = l*l*l, m*m*m, s*s*s
	return rgb{
		R: clamp01(linearToSrgb(+4.0767416621*l - 3.3077115913*m + 0.2309699292*s)),
		G: clamp01(linearToSrgb(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s)),
		B: clamp01(linearToSrgb(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)),
	}
}

func labDist(a, b lab) float64 {
	dl, da, db := a.L-b.L, a.A-b.A, a.B-b.B
	return math.Sqrt(dl*dl + da*da + db*db)
}

func toRGBA(c rgb) color.RGBA {
	return color.RGBA{
		R: uint8(math.Round(clamp01(c.R) * 255)),
		G: uint8(math.Round(clamp01(c.G) * 255)),
		B: uint8(math.Round(clamp01(c.B) * 255)),
		A: 255,
	}
}

func fromRGBA(c color.RGBA) rgb {
	return rgb{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255}
}
