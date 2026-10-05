package protocol

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// DefaultMaxPathBytes is the default limit on the length of a vault path.
const DefaultMaxPathBytes = 512

// NormalizePath converts a path as produced by a filesystem walk into wire
// form: forward slashes and Unicode NFC (macOS file systems produce NFD).
func NormalizePath(p string) string {
	return norm.NFC.String(strings.ReplaceAll(p, "\\", "/"))
}

// ValidatePath checks a vault-relative wire path (§3.4).
func ValidatePath(p string, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxPathBytes
	}
	switch {
	case p == "":
		return errors.New("empty path")
	case len(p) > maxBytes:
		return fmt.Errorf("path longer than %d bytes", maxBytes)
	case !utf8.ValidString(p):
		return errors.New("path is not valid UTF-8")
	case !norm.NFC.IsNormalString(p):
		return errors.New("path is not Unicode NFC")
	case strings.HasPrefix(p, "/"):
		return errors.New("path must be relative")
	case strings.Contains(p, "\\"):
		return errors.New("path must use forward slashes")
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return errors.New("path contains a control character")
		}
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return errors.New("path has an empty segment")
		case ".", "..":
			return errors.New("path has a dot segment")
		}
	}
	return nil
}

// IsReservedPath reports whether p is one of the reserved .kvist/ files.
func IsReservedPath(p string) bool {
	return p == HintsPath || p == SiteNotePath
}

// IsSettingsNote reports whether p is a settings note (_site.md, any case,
// in any folder).
func IsSettingsNote(p string) bool {
	return strings.EqualFold(path.Base(p), SettingsNoteName)
}

// IsNote reports whether p is a Markdown note.
func IsNote(p string) bool {
	return strings.EqualFold(path.Ext(p), ".md")
}

// Ext returns the lowercase extension of p without the dot.
func Ext(p string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(p), "."))
}

// AllowedPath reports whether p may appear in a manifest under rules r:
// notes, reserved files, and attachments with an allowed extension. Hidden
// files and folders (dot segments) are never allowed except the reserved
// files.
func AllowedPath(p string, r Rules) bool {
	if IsReservedPath(p) {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return false
		}
	}
	if IsNote(p) {
		return true
	}
	ext := Ext(p)
	for _, a := range r.AttachmentExtensions {
		if strings.EqualFold(strings.TrimPrefix(a, "."), ext) {
			return true
		}
	}
	return false
}
