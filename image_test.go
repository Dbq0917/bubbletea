package tea

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func decodeSixelImage(t *testing.T, output string) image.Image {
	t.Helper()

	start := strings.Index(output, "\x1bP")
	if start < 0 {
		t.Fatalf("no Sixel sequence in output %q", output)
	}
	q := strings.IndexByte(output[start:], 'q')
	if q < 0 {
		t.Fatalf("no Sixel introducer in output %q", output)
	}
	payloadStart := start + q + 1
	end := strings.Index(output[payloadStart:], "\x1b\\")
	if end < 0 {
		t.Fatalf("unterminated Sixel sequence in output %q", output)
	}

	return decodeSixelPayload(t, output[payloadStart:payloadStart+end])
}

func decodeSixelPayload(t *testing.T, payload string) image.Image {
	t.Helper()

	if len(payload) == 0 || payload[0] != '"' {
		t.Fatalf("missing raster attributes in %q", payload)
	}
	body := payload[1:]
	rest := ""
	if paletteAt := strings.IndexByte(body, '#'); paletteAt >= 0 {
		rest = body[paletteAt:]
		body = body[:paletteAt]
	}
	parts := strings.Split(body, ";")
	if len(parts) < 4 {
		t.Fatalf("malformed raster attributes in %q", payload)
	}
	width, err := strconv.Atoi(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	height, err := strconv.Atoi(parts[3])
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	palette := make(map[int]color.NRGBA)

	band := 0
	x := 0
	colorIndex := -1
	for i := 0; i < len(rest); {
		switch rest[i] {
		case '#':
			i++
			start := i
			for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
				i++
			}
			colorIndex, err = strconv.Atoi(rest[start:i])
			if err != nil {
				t.Fatal(err)
			}
			if i < len(rest) && rest[i] == ';' {
				// Palette definition: #idx;2;r;g;b
				fields := []int{}
				for i < len(rest) && rest[i] == ';' {
					i++
					start := i
					for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
						i++
					}
					n, err := strconv.Atoi(rest[start:i])
					if err != nil {
						t.Fatal(err)
					}
					fields = append(fields, n)
				}
				if len(fields) >= 4 {
					palette[colorIndex] = color.NRGBA{
						R: uint8(fields[1] * 255 / 100),
						G: uint8(fields[2] * 255 / 100),
						B: uint8(fields[3] * 255 / 100),
						A: 0xff,
					}
				}
			}
		case '$':
			x = 0
			i++
		case '-':
			band++
			x = 0
			i++
		case '!':
			i++
			start := i
			for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
				i++
			}
			count, err := strconv.Atoi(rest[start:i])
			if err != nil {
				t.Fatal(err)
			}
			if i >= len(rest) {
				t.Fatalf("missing repeated Sixel byte in %q", rest)
			}
			drawSixelByte(img, palette, colorIndex, x, band, rest[i], count)
			x += count
			i++
		default:
			drawSixelByte(img, palette, colorIndex, x, band, rest[i], 1)
			x++
			i++
		}
	}
	return img
}

func drawSixelByte(img *image.NRGBA, palette map[int]color.NRGBA, colorIndex, x, band int, b byte, count int) {
	c, ok := palette[colorIndex]
	if !ok {
		return
	}
	bits := (b - '?') & 0x3f
	for n := range count {
		for dy := range 6 {
			if bits&(1<<dy) == 0 {
				continue
			}
			y := band*6 + dy
			if px := x + n; px >= 0 && px < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.SetNRGBA(px, y, c)
			}
		}
	}
}

func TestSixelImageScalesByPixelGeometry(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	for y := range 100 {
		for x := range 400 {
			src.SetNRGBA(x, y, color.NRGBA{R: 0x33, G: 0x99, B: 0xcc, A: 0xff})
		}
	}

	tests := []struct {
		name      string
		cellW     int
		cellH     int
		bounds    Rectangle
		wantW     int
		wantH     int
		terminalW int
		terminalH int
	}{
		{
			name:      "laptop cells",
			cellW:     8,
			cellH:     16,
			bounds:    Rect(0, 0, 20, 10),
			wantW:     160,
			wantH:     40,
			terminalW: 640,
			terminalH: 320,
		},
		{
			name:      "4k cells",
			cellW:     16,
			cellH:     32,
			bounds:    Rect(0, 0, 20, 10),
			wantW:     320,
			wantH:     80,
			terminalW: 1280,
			terminalH: 640,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
			r.setPixelSize(tt.terminalW, tt.terminalH)
			r.setCellSize(tt.cellW, tt.cellH)
			r.setSixelSupport(true)

			view := NewView("status")
			view.Image = NewImage(src, tt.bounds)
			r.render(view)
			if _, err := r.flush(false); err != nil {
				t.Fatal(err)
			}

			got := decodeSixelImage(t, out.String())
			if got.Bounds().Dx() != tt.wantW || got.Bounds().Dy() != tt.wantH {
				t.Fatalf("image size = %dx%d, want %dx%d", got.Bounds().Dx(), got.Bounds().Dy(), tt.wantW, tt.wantH)
			}
			if ratio := float64(got.Bounds().Dx()) / float64(got.Bounds().Dy()); ratio < 3.9 || ratio > 4.1 {
				t.Fatalf("image aspect ratio = %.2f, want 4", ratio)
			}
		})
	}
}

func TestSixelImagePreservesTransparency(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 0xff, G: 0, B: 0, A: 0xff})
		}
	}

	encoded := encodeSixel(src)
	if !strings.HasPrefix(encoded, "\x1bP0;1q") {
		t.Fatalf("Sixel sequence does not request a transparent background: %q", encoded)
	}
	got := decodeSixelImage(t, encoded)
	for _, p := range []image.Point{{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 0, Y: 7}, {X: 7, Y: 7}} {
		_, _, _, a := got.At(p.X, p.Y).RGBA()
		if a != 0 {
			t.Fatalf("transparent pixel at %v has alpha %d, want 0", p, a)
		}
	}
	r, g, b, a := got.At(4, 4).RGBA()
	if a == 0 || r == 0 || g != 0 || b != 0 {
		t.Fatalf("opaque red pixel = (%d,%d,%d,%d), want opaque red", r, g, b, a)
	}
}

func TestHasSixel(t *testing.T) {
	t.Parallel()

	if !HasSixel([]int{1, 4, 6}) {
		t.Fatal("DA1 attribute 4 was not recognized as Sixel support")
	}
	if HasSixel([]int{1, 6, 22}) {
		t.Fatal("DA1 without attribute 4 was treated as Sixel support")
	}
}

func TestCursedRendererFallsBackWithoutSixel(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	src.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})

	var out bytes.Buffer
	r := newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	r.setPixelSize(800, 480)
	r.setCellSize(10, 20)
	r.setSixelSupport(false)

	view := NewView("character fallback")
	view.Image = NewImage(src, Rect(0, 0, 20, 10))
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if strings.Contains(got, "\x1bP") {
		t.Fatalf("fallback output contains a Sixel sequence: %q", got)
	}
	if !strings.Contains(got, "character fallback") {
		t.Fatalf("fallback content missing from output: %q", got)
	}
}

func TestCursedRendererFallsBackWhenCapabilityIsDisabled(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			src.SetNRGBA(x, y, color.NRGBA{B: 0xff, A: 0xff})
		}
	}

	var out bytes.Buffer
	r := newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	r.setPixelSize(800, 480)
	r.setCellSize(10, 20)
	r.setSixelSupport(true)

	view := NewView("image")
	view.Image = NewImage(src, Rect(0, 0, 20, 10))
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1bP") {
		t.Fatalf("expected an image while Sixel was enabled: %q", out.String())
	}

	mark := out.Len()
	r.setSixelSupport(false)
	view.Content = "fallback after disable"
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}

	got := out.String()[mark:]
	if strings.Contains(got, "\x1bP") {
		t.Fatalf("Sixel output remained after disabling support: %q", got)
	}
	if !strings.Contains(got, "fallback after disable") {
		t.Fatalf("character fallback was not rendered: %q", got)
	}
}

func TestCursedRendererDrawsImageBelowContent(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			src.SetNRGBA(x, y, color.NRGBA{R: 0x22, G: 0x44, B: 0x66, A: 0xff})
		}
	}

	var out bytes.Buffer
	r := newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	r.setPixelSize(800, 480)
	r.setCellSize(10, 20)
	r.setSixelSupport(true)

	view := NewView("status block")
	view.Image = NewImage(src, Rect(0, 0, 20, 10))
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	imageAt := strings.Index(got, "\x1bP")
	contentAt := strings.Index(got, "status block")
	if imageAt < 0 || contentAt < 0 {
		t.Fatalf("expected both image and content in output: %q", got)
	}
	if imageAt > contentAt {
		t.Fatalf("content must be emitted after the bitmap so overlays stay visible: %q", got)
	}
}

func TestCursedRendererDoesNotRedrawUnchangedImage(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			src.SetNRGBA(x, y, color.NRGBA{G: 0x80, A: 0xff})
		}
	}

	var out bytes.Buffer
	r := newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	r.setPixelSize(800, 480)
	r.setCellSize(10, 20)
	r.setSixelSupport(true)

	view := NewView("stable")
	view.Image = NewImage(src, Rect(0, 0, 20, 10))
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}
	first := out.String()
	if strings.Count(first, "\x1bP") != 1 {
		t.Fatalf("expected one image on first frame, got %q", first)
	}

	mark := out.Len()
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}
	if got := out.String()[mark:]; got != "" {
		t.Fatalf("unchanged image redrew the screen: %q", got)
	}

	next := *view.Image
	next.Revision++
	view.Image = &next
	r.render(view)
	if _, err := r.flush(false); err != nil {
		t.Fatal(err)
	}
	got := out.String()[mark:]
	if strings.Count(got, "\x1bP") != 1 {
		t.Fatalf("expected exactly one new image after invalidation, got %q", got)
	}
	imageAt := strings.Index(got, "\x1bP")
	contentAt := strings.Index(got, "stable")
	if contentAt < 0 || contentAt < imageAt {
		t.Fatalf("overlay was not re-emitted above the updated bitmap: %q", got)
	}
	if strings.Contains(got, ansi.EraseEntireScreen) || strings.Contains(got, ansi.EraseScreenBelow) {
		t.Fatalf("image invalidation caused a full-screen erase: %q", got)
	}
}

func TestProgramProbesGraphicsOnlyWhenImageIsUsed(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	model := &parkModel{view: NewView("no image")}
	p := NewProgram(model, WithInput(bytes.NewBuffer(nil)), WithOutput(&out))
	p.renderer = newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	p.ttyOutput = os.Stdin

	p.render(model)
	if p.graphicsProbed {
		t.Fatal("program probed graphics for a view without an image")
	}
	if got := p.outputBuf.String(); got != "" {
		t.Fatalf("unexpected terminal query without an image: %q", got)
	}

	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	model.view.Image = NewImage(src)
	p.render(model)
	if !p.graphicsProbed {
		t.Fatal("program did not probe graphics for a view with an image")
	}
	got := p.outputBuf.String()
	for _, want := range []string{
		ansi.RequestPrimaryDeviceAttributes,
		ansi.WindowOp(14),
		ansi.WindowOp(16),
		ansi.RequestModeSynchronizedOutput,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("graphics probe %q missing from %q", want, got)
		}
	}
}

func TestProgramDoesNotProbeGraphicsWithoutInput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	model := &parkModel{view: NewView("image")}
	model.view.Image = NewImage(src)
	p := NewProgram(model, WithInput(nil), WithOutput(&out))
	p.renderer = newCursedRenderer(&out, []string{"TERM=xterm-256color"}, 80, 24)
	p.ttyOutput = os.Stdin

	p.render(model)
	if !p.graphicsProbed {
		t.Fatal("program did not record the unavailable probe")
	}
	if got := p.outputBuf.String(); got != "" {
		t.Fatalf("terminal query was sent without input: %q", got)
	}
}
