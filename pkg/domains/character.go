package domains

import "encoding/xml"

// CharacterDevice omits host source paths: callers attach through libvirt, not
// by opening the host's PTY. A nil target port preserves an unspecified port.
type CharacterDevice struct {
	Type   string          `json:"type"`
	Alias  string          `json:"alias,omitempty"`
	Target CharacterTarget `json:"target"`
}

type CharacterTarget struct {
	Type string `json:"type,omitempty" xml:"type,attr"`
	Port *uint  `json:"port,omitempty" xml:"port,attr"`
}

func (d *CharacterDevice) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Type  string `xml:"type,attr"`
		Alias struct {
			Name string `xml:"name,attr"`
		} `xml:"alias"`
		Target CharacterTarget `xml:"target"`
	}
	if err := dec.DecodeElement(&raw, &start); err != nil {
		return err
	}
	*d = CharacterDevice{Type: raw.Type, Alias: raw.Alias.Name, Target: raw.Target}
	return nil
}
