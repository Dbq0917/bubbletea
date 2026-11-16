package tea

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// transparentAlpha is the cutoff below which a source pixel is left
// unpainted. Sixel has no alpha channel, so translucent pixels need a binary
// decision; keeping the threshold high avoids a dark halo around cutouts.
const transparentAlpha = 0x80

type sixelColor struct {
	r, g, b uint8
}

// encodeSixel encodes img as a transparent-background Sixel image. The
// returned string is a complete DCS sequence, including the introducer and
// string terminator.
func encodeSixel(img image.Image) string {
	if img == nil {
		return ""
	}
	bounds := img.Bounds()
	if bounds.Empty() {
		return ""
	}

	width, height := bounds.Dx(), bounds.Dy()
	palette, exact, transparent := sixelPalette(img)

	var payload strings.Builder
	// Raster attributes: 1:1 aspect ratio and the exact pixel dimensions.
	fmt.Fprintf(&payload, "\"1;1;%d;%d", width, height)

	for index, c := range palette {
		fmt.Fprintf(&payload, "#%d;2;%d;%d;%d", index,
			uint32(c.r)*100/255,
			uint32(c.g)*100/255,
			uint32(c.b)*100/255,
		)
	}

	indexAt := func(x, y int) (int, bool) {
		offset := y*bounds.Dx() + x
		if transparent[offset] {
			return 0, false
		}
		n := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
		c := sixelColor{r: n.R, g: n.G, b: n.B}
		if exact != nil {
			index, ok := exact[c]
			return index, ok
		}
		return sixelCubeIndex(c), true
	}

	for band := 0; band*6 < height; band++ {
		if band > 0 {
			payload.WriteByte('-')
		}

		// Build one mask row per used palette entry. Scanning the band once
		// keeps encoding O(width*height) even when the image has many colors.
		rows := make([][]byte, len(palette))
		for x := range width {
			for dy := range 6 {
				y := band*6 + dy
				if y >= height {
					break
				}
				index, opaque := indexAt(x, y)
				if !opaque {
					continue
				}
				if rows[index] == nil {
					rows[index] = make([]byte, width)
				}
				rows[index][x] |= 1 << dy
			}
		}

		firstColor := true
		var line strings.Builder
		for paletteIndex, row := range rows {
			if row == nil {
				continue
			}
			line.Reset()
			any := false
			for x := range width {
				bits := row[x]
				if bits != 0 {
					any = true
				}
				line.WriteByte('?' + bits)
			}
			if !any {
				continue
			}
			if !firstColor {
				payload.WriteByte('$')
			}
			firstColor = false
			payload.WriteByte('#')
			payload.WriteString(strconv.Itoa(paletteIndex))
			writeSixelRLE(&payload, line.String())
		}
	}

	return ansi.SixelGraphics(0, 1, 0, []byte(payload.String()))
}

// sixelPalette returns the opaque palette and a per-pixel transparency mask.
// Images with few colors keep their exact colors. Larger images use a compact
// 6x6x6 RGB cube, which is enough for monitoring charts and keeps encoding
// predictable at 4K sizes.
func sixelPalette(img image.Image) ([]sixelColor, map[sixelColor]int, []bool) {
	bounds := img.Bounds()
	pixelCount := bounds.Dx() * bounds.Dy()
	transparent := make([]bool, pixelCount)

	unique := make(map[sixelColor]struct{})
	fixed := false
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if c.A < transparentAlpha {
				transparent[y*bounds.Dx()+x] = true
				continue
			}
			if fixed {
				continue
			}
			unique[sixelColor{r: c.R, g: c.G, b: c.B}] = struct{}{}
			if len(unique) > 255 {
				fixed = true
				unique = nil
			}
		}
	}

	if fixed {
		colors := make([]sixelColor, 0, 216)
		for r := range 6 {
			for g := range 6 {
				for b := range 6 {
					colors = append(colors, sixelColor{
						r: uint8(r * 51),
						g: uint8(g * 51),
						b: uint8(b * 51),
					})
				}
			}
		}
		return colors, nil, transparent
	}

	colors := make([]sixelColor, 0, len(unique))
	for c := range unique {
		colors = append(colors, c)
	}
	slices.SortFunc(colors, func(a, b sixelColor) int {
		if a.r != b.r {
			return int(a.r) - int(b.r)
		}
		if a.g != b.g {
			return int(a.g) - int(b.g)
		}
		return int(a.b) - int(b.b)
	})
	exact := make(map[sixelColor]int, len(colors))
	for index, c := range colors {
		exact[c] = index
	}
	return colors, exact, transparent
}

func sixelCubeIndex(c sixelColor) int {
	// Round each channel to the nearest of six levels.
	r := (uint16(c.r)*5 + 127) / 255
	g := (uint16(c.g)*5 + 127) / 255
	b := (uint16(c.b)*5 + 127) / 255
	return int(r*36 + g*6 + b)
}

func writeSixelRLE(dst *strings.Builder, line string) {
	for i := 0; i < len(line); {
		j := i + 1
		for j < len(line) && line[j] == line[i] {
			j++
		}
		count := j - i
		if count > 3 {
			dst.WriteByte('!')
			dst.WriteString(strconv.Itoa(count))
			dst.WriteByte(line[i])
		} else {
			for range count {
				dst.WriteByte(line[i])
			}
		}
		i = j
	}
}
