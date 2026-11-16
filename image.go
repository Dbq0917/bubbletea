package tea

import (
	"image"
	"image/color"
	"math"
)

// Rectangle is a rectangle in terminal cell coordinates.
type Rectangle = image.Rectangle

// Rect returns a rectangle with the given origin and size.
func Rect(x, y, width, height int) Rectangle {
	return image.Rect(x, y, x+width, y+height)
}

// Image is a bitmap that can be displayed by a [View].
//
// Image rendering is optional and negotiated with the terminal at runtime.
// Terminals that do not support Sixel graphics simply keep rendering the
// regular [View.Content], so applications should keep a character-based
// fallback (for example, their existing Braille or block chart) in Content.
//
// Bounds is measured in terminal cells because that is the layout unit a
// Bubble Tea view knows about. The bitmap itself is sized from the terminal's
// pixel and cell dimensions, never by treating source pixels as cells. The
// image is scaled uniformly to fit Bounds and its aspect ratio is preserved.
//
// A zero Bounds means the whole frame. PixelWidth and PixelHeight can be used
// to impose an additional limit in terminal pixels. Transparent source pixels
// are left unpainted instead of being composited onto black.
//
// Image is intentionally independent from the Model, Cmd, and View method
// signatures. A View without an Image is rendered exactly as before.
type Image struct {
	// Image is the source bitmap. Source is an alias for it. Set either field;
	// Image takes precedence when both are non-nil.
	Image image.Image

	// Source is an alias for Image.
	Source image.Image

	// Bounds is the cell-space area in which the image may be drawn. A zero
	// rectangle means the whole view frame.
	Bounds Rectangle

	// PixelWidth and PixelHeight limit the rendered image size, in terminal
	// pixels. Zero means no additional limit beyond Bounds.
	PixelWidth  int
	PixelHeight int

	// Revision lets callers invalidate a cached rendering when they mutate the
	// pixels behind an existing Source pointer. Changing the source image or
	// creating a new Image also invalidates the cache.
	Revision uint64
}

// NewImage returns an image that occupies the whole view frame. An optional
// bounds argument limits it to a cell-space rectangle.
func NewImage(src image.Image, bounds ...Rectangle) *Image {
	im := &Image{Image: src, Source: src}
	if len(bounds) > 0 {
		im.Bounds = bounds[0]
	}
	return im
}

// NewImageIn returns an image constrained to the given cell-space rectangle.
func NewImageIn(src image.Image, bounds Rectangle) *Image {
	return NewImage(src, bounds)
}

// NewSixelImage is an alias for [NewImage].
func NewSixelImage(src image.Image, bounds ...Rectangle) *Image {
	return NewImage(src, bounds...)
}

// SetSource replaces the source bitmap.
func (i *Image) SetSource(src image.Image) {
	if i == nil {
		return
	}
	i.Image = src
	i.Source = src
}

// SetBounds sets the cell-space area the image may occupy.
func (i *Image) SetBounds(bounds Rectangle) {
	if i == nil {
		return
	}
	i.Bounds = bounds
}

// Refresh invalidates any cached rendering of the image. Call it after
// mutating pixels behind an existing source pointer.
func (i *Image) Refresh() {
	if i == nil {
		return
	}
	i.Revision++
}

func (i *Image) sourceImage() image.Image {
	if i == nil {
		return nil
	}
	if i.Image != nil {
		return i.Image
	}
	return i.Source
}

// fitIn returns the largest size that fits the source into maxWidth by
// maxHeight while preserving the source aspect ratio.
func fitIn(src image.Rectangle, maxWidth, maxHeight int) (width, height int) {
	if src.Empty() || maxWidth <= 0 || maxHeight <= 0 {
		return 0, 0
	}

	scale := math.Min(
		float64(maxWidth)/float64(src.Dx()),
		float64(maxHeight)/float64(src.Dy()),
	)
	if scale <= 0 {
		return 0, 0
	}

	width = max(1, int(math.Round(float64(src.Dx())*scale)))
	height = max(1, int(math.Round(float64(src.Dy())*scale)))
	width = min(width, maxWidth)
	height = min(height, maxHeight)
	return width, height
}

// targetSize computes the pixel size and cell-space origin for the image.
func (i *Image) targetSize(frame Rectangle, cellWidth, cellHeight int) (width, height int, origin image.Point, ok bool) {
	src := i.sourceImage()
	if src == nil || cellWidth <= 0 || cellHeight <= 0 {
		return 0, 0, image.Point{}, false
	}

	bounds := i.Bounds
	if bounds.Empty() {
		bounds = frame
	}
	bounds = bounds.Intersect(frame)
	if bounds.Empty() {
		return 0, 0, image.Point{}, false
	}

	maxWidth := bounds.Dx() * cellWidth
	maxHeight := bounds.Dy() * cellHeight
	if i.PixelWidth > 0 {
		maxWidth = min(maxWidth, i.PixelWidth)
	}
	if i.PixelHeight > 0 {
		maxHeight = min(maxHeight, i.PixelHeight)
	}

	width, height = fitIn(src.Bounds(), maxWidth, maxHeight)
	if width == 0 || height == 0 {
		return 0, 0, image.Point{}, false
	}
	return width, height, bounds.Min, true
}

// scaleImage scales src to width by height. The implementation uses nearest
// neighbor sampling, which keeps the dependency footprint small and preserves
// transparent pixels exactly.
func scaleImage(src image.Image, width, height int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	srcBounds := src.Bounds()
	if srcBounds.Empty() || width <= 0 || height <= 0 {
		return dst
	}

	for y := range height {
		sy := srcBounds.Min.Y + int(float64(y)*float64(srcBounds.Dy())/float64(height))
		sy = min(sy, srcBounds.Max.Y-1)
		for x := range width {
			sx := srcBounds.Min.X + int(float64(x)*float64(srcBounds.Dx())/float64(width))
			sx = min(sx, srcBounds.Max.X-1)
			dst.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA))
		}
	}
	return dst
}
