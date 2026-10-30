package main

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func luminance(hex string) float64 {
	value, _ := strconv.ParseUint(hex[1:], 16, 32)
	channel := func(v uint64) float64 {
		x := float64(v) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return math.Pow((x+.055)/1.055, 2.4)
	}
	return .2126*channel(value>>16) + .7152*channel((value>>8)&255) + .0722*channel(value&255)
}
func contrast(a, b string) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + .05) / (y + .05)
}
func themeHex(role string, dark bool) string {
	color := themeColor(role)
	if a, ok := color.(lipgloss.AdaptiveColor); ok {
		if dark {
			return a.Dark
		}
		return a.Light
	}
	return string(color.(lipgloss.Color))
}
func TestThemeContrastTruecolorAndStandard256Palette(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256} {
			convert := func(hex string) string { return fmt.Sprint(profile.Color(hex)) }
			base := themeHex("canvas", dark)
			group, selected := "#f0f0f0", themeHex("selection", dark)
			if dark {
				group = "#292e34"
			}
			minimum := 100.0
			for _, role := range []string{"1", "2", "3", "6", "7", "8"} {
				for _, bg := range []string{base, group, selected} {
					ratio := contrast(convert(themeHex(role, dark)), convert(bg))
					minimum = min(minimum, ratio)
					if ratio < 4.5 {
						t.Errorf("text role %s dark=%v profile=%v on %s: %.2f < 4.5", role, dark, profile, bg, ratio)
					}
				}
			}
			track, thumb := themeHex("scroll-track", dark), themeHex("7", dark)
			for _, pair := range [][2]string{{track, base}, {thumb, base}, {thumb, track}} {
				ratio := contrast(convert(pair[0]), convert(pair[1]))
				if ratio < 3 {
					t.Errorf("scrollbar dark=%v profile=%v %v: %.2f < 3", dark, profile, pair, ratio)
				}
			}
			t.Logf("dark=%v profile=%v minimum text contrast %.2f:1", dark, profile, minimum)
		}
	}
}
