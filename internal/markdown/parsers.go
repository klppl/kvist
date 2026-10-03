package markdown

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// --- [[wikilinks]] and ![[embeds]] ---

type wikiLinkParser struct{}

func (wikiLinkParser) Trigger() []byte { return []byte{'!', '['} }

func (wikiLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	i := 0
	embed := false
	if len(line) > 0 && line[0] == '!' {
		embed, i = true, 1
	}
	if len(line) < i+5 || line[i] != '[' || line[i+1] != '[' {
		return nil
	}
	end := bytes.Index(line[i+2:], []byte("]]"))
	if end <= 0 {
		return nil
	}
	inner := string(line[i+2 : i+2+end])
	if strings.ContainsAny(inner, "[\n") {
		return nil
	}
	n := &WikiLink{Embed: embed}
	target, alias, hasAlias := strings.Cut(inner, "|")
	if hasAlias {
		target = strings.TrimSuffix(target, `\`) // [[a\|b]] inside tables
		n.Alias = strings.TrimSpace(alias)
	}
	target, sub, _ := strings.Cut(target, "#")
	n.Target, n.Subpath = strings.TrimSpace(target), strings.TrimSpace(sub)
	if n.Target == "" && n.Subpath == "" {
		return nil
	}
	block.Advance(i + 2 + end + 2)
	return n
}

// --- #tags ---

type tagParser struct{}

func (tagParser) Trigger() []byte { return []byte{'#'} }

func (tagParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if before := block.PrecendingCharacter(); !unicode.IsSpace(before) {
		return nil
	}
	line, _ := block.PeekLine()
	j := 1
	for j < len(line) {
		r, size := utf8.DecodeRune(line[j:])
		if !isTagRune(r) {
			break
		}
		j += size
	}
	name := strings.TrimRight(string(line[1:j]), "/")
	if !validTag(name) {
		return nil
	}
	block.Advance(1 + len(name))
	return &Tag{Name: name}
}

func isTagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' || r == '-' || r == '/'
}

func validTag(t string) bool {
	nonDigit := false
	for _, r := range t {
		if !isTagRune(r) {
			return false
		}
		if !unicode.IsDigit(r) {
			nonDigit = true
		}
	}
	return nonDigit
}

// --- ==highlight== ---

type highlightDelimiter struct{}

func (highlightDelimiter) IsDelimiter(b byte) bool { return b == '=' }

func (highlightDelimiter) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (highlightDelimiter) OnMatch(consumes int) ast.Node { return &Highlight{} }

type highlightParser struct{}

func (highlightParser) Trigger() []byte { return []byte{'='} }

func (highlightParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()
	node := parser.ScanDelimiter(line, before, 2, highlightDelimiter{})
	if node == nil || node.OriginalLength != 2 || before == '=' {
		return nil
	}
	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

// --- $math$ ---

type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if bytes.HasPrefix(line, []byte("$$")) {
		end := bytes.Index(line[2:], []byte("$$"))
		if end <= 0 {
			return nil
		}
		block.Advance(2 + end + 2)
		return &Math{TeX: string(line[2 : 2+end]), Display: true}
	}
	// $x$: no space after the opening or before the closing dollar, and the
	// closing dollar not followed by a digit (so "$5 and $6" stays text).
	if len(line) < 3 || line[1] == ' ' || line[1] == '\t' {
		return nil
	}
	for k := 1; k < len(line); k++ {
		switch line[k] {
		case '\\':
			k++
		case '$':
			if line[k-1] == ' ' || line[k-1] == '\t' {
				continue
			}
			if k+1 < len(line) && line[k+1] >= '0' && line[k+1] <= '9' {
				continue
			}
			block.Advance(k + 1)
			return &Math{TeX: string(line[1:k])}
		case '\n':
			return nil
		}
	}
	return nil
}

// --- $$ math blocks $$ ---

type mathBlockParser struct{}

var mathBlockOpen = parser.NewContextKey()

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	rest := bytes.TrimSpace(line[pos+2:])
	node := &MathBlock{}
	if bytes.HasSuffix(rest, []byte("$$")) && len(rest) >= 2 {
		// Single line: $$ x $$.
		inner := bytes.TrimSuffix(rest, []byte("$$"))
		start := segment.Start + pos + 2 + bytes.Index(line[pos+2:], inner)
		if len(inner) > 0 {
			node.Lines().Append(text.NewSegment(start, start+len(inner)))
		}
		reader.Advance(segment.Len() - 1)
		return node, parser.Close
	}
	if len(rest) > 0 {
		start := segment.Start + pos + 2 + bytes.Index(line[pos+2:], rest)
		node.Lines().Append(text.NewSegment(start, start+len(rest)))
	}
	reader.Advance(segment.Len() - 1)
	pc.Set(mathBlockOpen, node)
	return node, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if line == nil {
		return parser.Close
	}
	trimmed := bytes.TrimSpace(line)
	if bytes.HasSuffix(trimmed, []byte("$$")) {
		inner := bytes.TrimSuffix(trimmed, []byte("$$"))
		if len(inner) > 0 {
			start := segment.Start + bytes.Index(line, inner)
			node.Lines().Append(text.NewSegment(start, start+len(inner)))
		}
		reader.Advance(segment.Len() - 1)
		return parser.Close
	}
	node.Lines().Append(segment)
	reader.Advance(segment.Len() - 1)
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	pc.Set(mathBlockOpen, nil)
}

func (mathBlockParser) CanInterruptParagraph() bool { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

func inlineParsers() []util.PrioritizedValue {
	return []util.PrioritizedValue{
		util.Prioritized(wikiLinkParser{}, 199), // before the link parser (200)
		util.Prioritized(mathInlineParser{}, 150),
		util.Prioritized(tagParser{}, 600),
		util.Prioritized(highlightParser{}, 500),
	}
}

func blockParsers() []util.PrioritizedValue {
	return []util.PrioritizedValue{
		util.Prioritized(mathBlockParser{}, 690), // before fenced code (700)
	}
}
