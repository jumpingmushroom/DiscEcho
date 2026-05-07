package identify

import (
	"encoding/xml"
	"strings"
)

// ParseBDMT extracts the human-readable disc title from a BDMV disc's
// `BDMV/META/DL/bdmt_<lang>.xml` file. Returns "" on empty/malformed
// input or when no <di:name> element is found inside a <di:title>.
// Never panics.
//
// The format uses the BDA disclib + discinfo namespaces; we ignore
// namespaces and look for any element named "name" nested inside an
// element named "title" — robust to namespace-prefix variation across
// encoders.
func ParseBDMT(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false

	var (
		inTitle bool
		depth   int
		titleAt int
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "title" {
				inTitle = true
				titleAt = depth
			}
			if inTitle && t.Name.Local == "name" {
				var content struct {
					XMLName xml.Name
					Inner   string `xml:",chardata"`
				}
				if err := dec.DecodeElement(&content, &t); err != nil {
					return ""
				}
				name := strings.TrimSpace(content.Inner)
				if name != "" {
					return name
				}
				// DecodeElement consumed the EndElement; replicate the
				// depth bookkeeping the parent loop would have done.
				depth--
			}
		case xml.EndElement:
			if inTitle && depth == titleAt {
				inTitle = false
			}
			depth--
		}
	}
}
