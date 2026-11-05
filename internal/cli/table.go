package cli

import (
	"bytes"
	"io"
	"strings"
	"unicode"
)

// table is a minimal tabwriter replacement that aligns by terminal display
// width, so CJK and other wide characters (2 columns each) line up.
// Cells are separated by '\t', rows by '\n'; the last cell is not padded.
type table struct {
	w   io.Writer
	buf bytes.Buffer
}

func newTable(w io.Writer) *table { return &table{w: w} }

func (t *table) Write(p []byte) (int, error) { return t.buf.Write(p) }

func (t *table) Flush() error {
	text := strings.TrimSuffix(t.buf.String(), "\n")
	t.buf.Reset()
	if text == "" {
		return nil
	}
	var rows [][]string
	var widths []int
	for _, line := range strings.Split(text, "\n") {
		cells := strings.Split(line, "\t")
		rows = append(rows, cells)
		for i, c := range cells[:len(cells)-1] {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], displayWidth(c))
		}
	}
	var out strings.Builder
	for _, cells := range rows {
		for i, c := range cells {
			out.WriteString(c)
			if i < len(cells)-1 {
				out.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		out.WriteByte('\n')
	}
	_, err := io.WriteString(t.w, out.String())
	return err
}

// displayWidth approximates the terminal width of s: East Asian wide and
// fullwidth characters count 2, combining marks 0.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '‍' || (r >= 0xfe00 && r <= 0xfe0f):
		case isWide(r):
			n += 2
		default:
			n++
		}
	}
	return n
}

func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115f) || // Hangul Jamo
		(r >= 0x2e80 && r <= 0x303e) || // CJK radicals, punctuation
		(r >= 0x3041 && r <= 0x33ff) || // kana, CJK compatibility
		(r >= 0x3400 && r <= 0x4dbf) || // CJK ext A
		(r >= 0x4e00 && r <= 0x9fff) || // CJK unified
		(r >= 0xa000 && r <= 0xa4cf) || // Yi
		(r >= 0xac00 && r <= 0xd7a3) || // Hangul syllables
		(r >= 0xf900 && r <= 0xfaff) || // CJK compatibility ideographs
		(r >= 0xfe30 && r <= 0xfe4f) || // CJK compatibility forms
		(r >= 0xff00 && r <= 0xff60) || // fullwidth forms
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1f64f) || // emoji
		(r >= 0x1f900 && r <= 0x1f9ff) ||
		(r >= 0x20000 && r <= 0x3fffd) // CJK ext B+
}
