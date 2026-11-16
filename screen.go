package tea

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// WindowSizeMsg is used to report the terminal size. It's sent to Update once
// initially and then on every terminal resize.
type WindowSizeMsg struct {
	Width  int
	Height int
}

// PixelSizeMsg reports the terminal window size in pixels.
//
// The size is obtained from the terminal at runtime, because a local process
// connected over SSH cannot infer the graphics capabilities or pixel geometry
// of the terminal on the other end from environment variables alone.
type PixelSizeMsg = uv.PixelSizeEvent

// CellSizeMsg reports the size of one terminal cell in pixels.
type CellSizeMsg = uv.CellSizeEvent

// PrimaryDeviceAttributesMsg is the terminal's response to a Primary Device
// Attributes (DA1) request. Attribute 4 advertises Sixel graphics support.
type PrimaryDeviceAttributesMsg = uv.PrimaryDeviceAttributesEvent

// HasSixel reports whether Primary Device Attributes advertised Sixel
// graphics support.
func HasSixel(attributes []int) bool {
	for _, attr := range attributes {
		if attr == 4 {
			return true
		}
	}
	return false
}

// GraphicsCapabilityMsg reports a runtime graphics capability decision to a
// Program. Bubble Tea sends it after the terminal answers the graphics probe,
// so a model can switch between its character fallback and bitmap view.
// Programs normally do not need to construct this message; it is also useful
// for tests and for integrations that perform their own terminal capability
// negotiation.
type GraphicsCapabilityMsg struct {
	// Sixel reports whether Sixel graphics may be used.
	Sixel bool
}

// SixelSupportMsg is an explicit Sixel capability report. A true value
// enables image rendering; a false value forces the character fallback.
type SixelSupportMsg struct {
	Supported bool
}

// ClearScreen is a special command that tells the program to clear the screen
// before the next update. This can be used to move the cursor to the top left
// of the screen and clear visual clutter when the alt screen is not in use.
//
// Note that it should never be necessary to call ClearScreen() for regular
// redraws.
func ClearScreen() Msg {
	return clearScreenMsg{}
}

// clearScreenMsg is an internal message that signals to clear the screen.
// You can send a clearScreenMsg with ClearScreen.
type clearScreenMsg struct{}

// ModeReportMsg is a message that represents a mode report event (DECRPM).
//
// This is sent by the terminal in response to a request for a terminal mode
// report (DECRQM). It indicates the current setting of a specific terminal
// mode like cursor visibility, mouse tracking, etc.
//
// Example:
//
//	```go
//	func (m model) Init() tea.Cmd {
//	  // Does my terminal support reporting focus events?
//	  return tea.Raw(ansi.RequestModeFocusEvent)
//	}
//
//	func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//	  switch msg := msg.(type) {
//	  case tea.ModeReportMsg:
//	    if msg.Mode == ansi.ModeFocusEvent && !msg.Value.IsNotRecognized() {
//	      // Terminal supports focus events
//	      m.supportsFocus = true
//	    }
//	  }
//	  return m, nil
//	}
//
//	func (m model) View() tea.View {
//	  var view tea.View
//	  view.ReportFocus = m.supportsFocus
//	  view.SetContent(fmt.Sprintf("Terminal supports focus events: %v", m.supportsFocus))
//	  return view
//	}
//	```
//
// See: https://vt100.net/docs/vt510-rm/DECRPM.html
type ModeReportMsg struct {
	// Mode is the mode number.
	Mode ansi.Mode

	// Value is the mode value.
	Value ansi.ModeSetting
}

const (
	maxWindowDimension = 4096
	maxWindowCells     = 1 << 20
)

func isPlausibleWindowSize(width, height int) bool {
	if width < 0 || height < 0 {
		return false
	}
	if width > maxWindowDimension || height > maxWindowDimension {
		return false
	}
	return width*height <= maxWindowCells
}
