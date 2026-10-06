package pushclient

import (
	"bytes"
	"encoding/json"
	"strings"
)

// stripCanvas removes the file cards of a canvas whose file is not going
// up, and the arrows to and from them, so a canvas never carries the vault
// paths of private notes or files to the server. keep reports whether a
// card's file goes up. A canvas with nothing to remove comes back as it is;
// otherwise it is re-encoded with sorted keys and no spaces, as the plugin
// does. ok is false when src isn't a JSON object.
func stripCanvas(src []byte, keep func(file string) bool) (out []byte, ok bool) {
	var doc map[string]any
	if err := json.Unmarshal(src, &doc); err != nil || doc == nil {
		return nil, false
	}
	nodes, _ := doc["nodes"].([]any)
	dropped := map[string]bool{}
	kept := make([]any, 0, len(nodes))
	for _, n := range nodes {
		m, _ := n.(map[string]any)
		file, _ := m["file"].(string)
		if m != nil && m["type"] == "file" && strings.TrimSpace(file) != "" && !keep(file) {
			if id, ok := m["id"].(string); ok {
				dropped[id] = true
			}
			continue
		}
		kept = append(kept, n)
	}
	if len(kept) == len(nodes) {
		return src, true
	}
	doc["nodes"] = kept
	if edges, ok := doc["edges"].([]any); ok {
		keptEdges := make([]any, 0, len(edges))
		for _, e := range edges {
			m, _ := e.(map[string]any)
			from, _ := m["fromNode"].(string)
			to, _ := m["toNode"].(string)
			if m != nil && (dropped[from] || dropped[to]) {
				continue
			}
			keptEdges = append(keptEdges, e)
		}
		doc["edges"] = keptEdges
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // like JSON.stringify
	if err := enc.Encode(doc); err != nil {
		return nil, false
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), true
}
