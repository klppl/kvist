package vault

import (
	"encoding/json"
	"strings"
)

// Canvas is an Obsidian canvas (a .canvas file, JSON Canvas 1.0): cards
// laid out on a board and the arrows between them.
type Canvas struct {
	Nodes []CanvasNode `json:"nodes"`
	Edges []CanvasEdge `json:"edges"`
}

// CanvasNode is one card. Type is "text", "file", "link" or "group".
type CanvasNode struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Color  string  `json:"color,omitempty"`

	Text    string `json:"text,omitempty"`    // text: Markdown
	File    string `json:"file,omitempty"`    // file: vault path
	Subpath string `json:"subpath,omitempty"` // file: "#Heading" or "#^block"
	URL     string `json:"url,omitempty"`     // link
	Label   string `json:"label,omitempty"`   // group
}

// CanvasEdge is an arrow between two cards. Sides are "top", "right",
// "bottom" or "left"; ends are "none" or "arrow" (by default the arrow
// points at ToNode only).
type CanvasEdge struct {
	ID       string `json:"id"`
	FromNode string `json:"fromNode"`
	FromSide string `json:"fromSide,omitempty"`
	FromEnd  string `json:"fromEnd,omitempty"`
	ToNode   string `json:"toNode"`
	ToSide   string `json:"toSide,omitempty"`
	ToEnd    string `json:"toEnd,omitempty"`
	Color    string `json:"color,omitempty"`
	Label    string `json:"label,omitempty"`
}

// IsCanvas reports whether p is a canvas file.
func IsCanvas(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".canvas")
}

// ParseCanvas decodes a canvas file.
func ParseCanvas(src []byte) (*Canvas, error) {
	var c Canvas
	if err := json.Unmarshal(src, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Links lists what the canvas points to: each file card as an embed of its
// file, then the links and embeds in each text card (outside code and
// comments, as in notes). Offset is the index of the card.
func (c *Canvas) Links() []Link {
	var out []Link
	for i, n := range c.Nodes {
		switch n.Type {
		case "file":
			if strings.TrimSpace(n.File) != "" {
				out = append(out, Link{Target: n.File, Subpath: strings.TrimPrefix(n.Subpath, "#"), Embed: true, Offset: i})
			}
		case "text":
			for _, l := range scanLinks(maskComments(maskCode([]byte(n.Text))), 0) {
				l.Offset = i
				out = append(out, l)
			}
		}
	}
	return out
}
