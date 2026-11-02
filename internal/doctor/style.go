package doctor

// style paints doctor output with ANSI SGR codes when on. With on false every
// method returns its input unchanged, so plain output stays byte-for-byte the
// same. Color is decided once by the command (listing.ShouldUseColor), which
// honours NO_COLOR and a non-terminal stdout.
type style struct{ on bool }

const (
	sgrRed    = "31"
	sgrGreen  = "32"
	sgrYellow = "33"
	sgrReset  = "\x1b[0m"
)

func (s style) paint(code, text string) string {
	if !s.on {
		return text
	}
	return "\x1b[" + code + "m" + text + sgrReset
}

// status colors a check verdict: OK green, WARN yellow, FAULT and ERROR red.
// SKIP stays plain.
func (s style) status(st Status) string {
	switch st {
	case OK:
		return s.paint(sgrGreen, string(st))
	case WARN:
		return s.paint(sgrYellow, string(st))
	case FAULT, ERROR:
		return s.paint(sgrRed, string(st))
	default:
		return string(st)
	}
}

// style returns the painter for this doctor's output.
func (d HostDoctor) style() style { return style{on: d.Color} }

// notice colors a line of the cost warning shown before a repair applies.
func (s style) notice(text string) string {
	return s.paint(sgrYellow, text)
}
