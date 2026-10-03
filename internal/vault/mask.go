package vault

import (
	"bytes"
	"regexp"
	"strings"
)

// maskCode returns a copy of body where fenced code blocks, indented code
// blocks and inline code spans are replaced by spaces. Newlines and byte
// offsets are preserved.
func maskCode(body []byte) []byte {
	out := append([]byte(nil), body...)
	maskCodeBlocks(out)
	maskCodeSpans(out)
	return out
}

var (
	listItemRe = regexp.MustCompile(`^\s*([-*+]|\d{1,9}[.)])(\s|$)`)
	fenceRe    = regexp.MustCompile("^(`{3,}|~{3,})")
)

// stripQuote removes blockquote/callout markers ("> > ") from a line.
func stripQuote(line string) (rest string, quoted bool) {
	for {
		t := strings.TrimLeft(line, " ")
		if len(line)-len(t) > 3 || !strings.HasPrefix(t, ">") {
			return line, quoted
		}
		line = strings.TrimPrefix(t[1:], " ")
		quoted = true
	}
}

func indentWidth(line string) int {
	w := 0
	for _, c := range line {
		switch c {
		case ' ':
			w++
		case '\t':
			w += 4 - w%4
		default:
			return w
		}
	}
	return w
}

func blank(b []byte) {
	for i := range b {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

func maskCodeBlocks(b []byte) {
	var (
		fence      string // open fence marker, "" when not in a fence
		prevBlank  = true
		inIndented bool
		inList     bool
	)
	off := 0
	for off < len(b) {
		end := bytes.IndexByte(b[off:], '\n')
		if end < 0 {
			end = len(b)
		} else {
			end += off
		}
		raw := string(b[off:end])
		line, quoted := stripQuote(strings.TrimRight(raw, "\r"))
		trimmed := strings.TrimSpace(line)
		isBlank := trimmed == ""

		switch {
		case fence != "":
			// Inside a fence: blank the line; a closing fence of the same
			// character and at least the same length ends it.
			blank(b[off:end])
			if m := fenceRe.FindString(trimmed); m != "" && m[0] == fence[0] &&
				len(m) >= len(fence) && strings.TrimLeft(trimmed, m[:1]) == "" {
				fence = ""
			}
		case fenceRe.MatchString(strings.TrimLeft(line, " \t")):
			m := fenceRe.FindString(strings.TrimLeft(line, " \t"))
			// A backtick fence's info string may not contain backticks.
			if m[0] == '`' && strings.Contains(strings.TrimLeft(line, " \t")[len(m):], "`") {
				break
			}
			fence = m
			inIndented = false
			blank(b[off:end])
		case !isBlank && !quoted && indentWidth(line) >= 4 && (inIndented || prevBlank) && !inList:
			inIndented = true
			blank(b[off:end])
		default:
			if !isBlank {
				inIndented = false
				switch {
				case listItemRe.MatchString(line):
					inList = true
				case indentWidth(line) == 0 && prevBlank:
					inList = false
				}
			}
		}
		prevBlank = isBlank
		off = end + 1
	}
}

// maskCodeSpans blanks inline `code` spans. A span opened by a run of N
// backticks closes at the next run of exactly N backticks within the same
// paragraph; an unmatched run is literal text.
func maskCodeSpans(b []byte) {
	for i := 0; i < len(b); {
		if b[i] == '\\' {
			i += 2
			continue
		}
		if b[i] != '`' {
			i++
			continue
		}
		n := runLen(b, i, '`')
		closeAt := -1
		for j := i + n; j < len(b); {
			if b[j] == '\n' && j+1 < len(b) && isBlankLineAt(b, j+1) {
				break
			}
			if b[j] == '`' {
				m := runLen(b, j, '`')
				if m == n {
					closeAt = j
					break
				}
				j += m
				continue
			}
			j++
		}
		if closeAt < 0 {
			i += n
			continue
		}
		blank(b[i : closeAt+n])
		i = closeAt + n
	}
}

func runLen(b []byte, i int, c byte) int {
	n := 0
	for i+n < len(b) && b[i+n] == c {
		n++
	}
	return n
}

func isBlankLineAt(b []byte, i int) bool {
	for ; i < len(b) && b[i] != '\n'; i++ {
		if b[i] != ' ' && b[i] != '\t' && b[i] != '\r' {
			return false
		}
	}
	return true
}

// maskComments returns a copy of b with %%…%% and <!-- … --> comments
// blanked. An unclosed comment runs to the end of the note, as in Obsidian.
func maskComments(b []byte) []byte {
	out := append([]byte(nil), b...)
	for i := 0; i < len(out); i++ {
		var closer string
		var open int
		switch {
		case bytes.HasPrefix(out[i:], []byte("%%")):
			closer, open = "%%", 2
		case bytes.HasPrefix(out[i:], []byte("<!--")):
			closer, open = "-->", 4
		default:
			continue
		}
		end := bytes.Index(out[i+open:], []byte(closer))
		stop := len(out)
		if end >= 0 {
			stop = i + open + end + len(closer)
		}
		blank(out[i:stop])
		i = stop - 1
	}
	return out
}
