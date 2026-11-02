package main

import "math"

// constrainChromaError keeps diffusion from turning a faint tint into isolated
// saturated pixels in bright haze. The source, before accumulated error,
// determines how much chromatic error can be received. Luminance error is
// preserved for shading; midtones are unaffected to retain subtle skin colors.
// C* <= 12 is near-neutral; the smooth transition ends at C* = 16 so soft skin
// tones and colored details retain ordinary color diffusion.
func constrainChromaError(source, adjusted [3]float64) [3]float64 {
	l, a, b := rgbToLab(source[0], source[1], source[2])
	chroma := math.Hypot(a, b)
	if chroma >= 16 || l <= 80 {
		return adjusted
	}
	strength := math.Max(0, (chroma-12)/4)
	strength = strength * strength * (3 - 2*strength)
	brightness := math.Min(1, (l-80)/8)
	brightness = brightness * brightness * (3 - 2*brightness)
	strength = 1 - brightness*(1-strength)
	err := [3]float64{adjusted[0] - source[0], adjusted[1] - source[1], adjusted[2] - source[2]}
	lumaError := 0.2126*err[0] + 0.7152*err[1] + 0.0722*err[2]
	for channel := range adjusted {
		adjusted[channel] = source[channel] + lumaError + strength*(err[channel]-lumaError)
	}
	return adjusted
}
