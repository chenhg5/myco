package oscap

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// xmlPlistStrings reads the top-level string values of an XML property
// list (an app's Info.plist).
func xmlPlistStrings(b []byte) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	depth, key, inKey, inString := 0, "", false, false
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			text.Reset()
			// (plist > dict is depth 2; its keys and values are at 3)
			inKey = depth == 3 && t.Name.Local == "key"
			inString = depth == 3 && t.Name.Local == "string"
		case xml.CharData:
			if inKey || inString {
				text.Write(t)
			}
		case xml.EndElement:
			if inKey {
				key = strings.TrimSpace(text.String())
			} else if inString && key != "" {
				out[key] = strings.TrimSpace(text.String())
				key = ""
			} else if depth == 3 {
				key = "" // (a value that is not a string)
			}
			inKey, inString = false, false
			depth--
		}
	}
}
