// Package publish implements the note-level publish rules (§5.1). The server
// runs them at commit and at build time (gate 2); the reference push client
// runs them before pushing (gate 1).
package publish

import (
	"strings"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/vault"
)

// Reasons explain a decision. They are shown in leak reports and warnings.
const (
	ReasonExcludedFolder     = "excluded_folder"
	ReasonFrontmatterFalse   = "frontmatter_false"
	ReasonPrivateTag         = "private_tag"
	ReasonInvalidFrontmatter = "invalid_frontmatter"
	ReasonPublicFolder       = "public_folder"
	ReasonPublicTag          = "public_tag"
	ReasonFrontmatterTrue    = "frontmatter_true"
	ReasonNoRule             = "no_rule"
	ReasonNotANote           = "not_a_note"
	ReasonSettingsNote       = "settings_note"
)

// Decision is the outcome of evaluating the rules for one note.
type Decision struct {
	Published bool
	Reason    string
}

// Evaluate applies the rules to a note at vault path p with metadata m.
// Order (first match wins):
//
//  0. excluded if it is the settings note (_site.md): it configures the
//     site and is never a page;
//  1. excluded if inside an exclude_folders entry;
//  2. excluded if frontmatter <key>: false, or tagged with the private tag,
//     or the frontmatter cannot be read (fail closed);
//  3. included if inside an always_public_folders entry, tagged with the
//     public tag, or frontmatter <key>: true;
//  4. excluded otherwise.
func Evaluate(r protocol.Rules, p string, m *vault.Meta) Decision {
	if !protocol.IsNote(p) {
		return Decision{false, ReasonNotANote}
	}
	if protocol.IsSettingsNote(p) {
		return Decision{false, ReasonSettingsNote}
	}
	for _, f := range r.ExcludeFolders {
		if inFolder(p, f, true) {
			return Decision{false, ReasonExcludedFolder}
		}
	}
	if m.FrontmatterErr != nil {
		return Decision{false, ReasonInvalidFrontmatter}
	}
	fm := frontmatterFlag(m.Frontmatter, r.FrontmatterKey)
	if fm == flagFalse {
		return Decision{false, ReasonFrontmatterFalse}
	}
	if r.PrivateTag != "" && m.HasTag(r.PrivateTag) {
		return Decision{false, ReasonPrivateTag}
	}
	for _, f := range r.AlwaysPublicFolders {
		if inFolder(p, f, false) {
			return Decision{true, ReasonPublicFolder}
		}
	}
	if r.PublicTag != "" && m.HasTag(r.PublicTag) {
		return Decision{true, ReasonPublicTag}
	}
	if fm == flagTrue {
		return Decision{true, ReasonFrontmatterTrue}
	}
	return Decision{false, ReasonNoRule}
}

// AttachmentAllowed reports whether an attachment may be published at all.
// Attachments have no rules of their own (a published note must reference
// them), but nothing inside an exclude_folders entry is ever published.
func AttachmentAllowed(r protocol.Rules, p string) bool {
	for _, f := range r.ExcludeFolders {
		if inFolder(p, f, true) {
			return false
		}
	}
	return true
}

// CanvasInPublicFolder reports whether the canvas at p is published on its
// own, without a published note linking to it: it lies in an
// always_public_folders entry and in no exclude_folders entry. Canvases
// have no frontmatter or tags, so the folder is their only rule; any other
// canvas is published like an attachment, when a published note (or a
// published canvas) links to or embeds it.
func CanvasInPublicFolder(r protocol.Rules, p string) bool {
	if !vault.IsCanvas(p) || !protocol.AllowedPath(p, r) || !AttachmentAllowed(r, p) {
		return false
	}
	for _, f := range r.AlwaysPublicFolders {
		if inFolder(p, f, false) {
			return true
		}
	}
	return false
}

// EvaluateSource parses src and evaluates the rules for it.
func EvaluateSource(r protocol.Rules, p string, src []byte) (Decision, *vault.Meta) {
	m := vault.ParseMeta(src)
	return Evaluate(r, p, m), m
}

// inFolder reports whether p lies inside folder f ("/" is the vault root).
// Exclusions match case-insensitively so a differently cased folder name
// can't slip past them; inclusions match exactly.
func inFolder(p, f string, foldCase bool) bool {
	if f == "/" {
		return true
	}
	f = strings.Trim(f, "/")
	if f == "" {
		return false
	}
	if foldCase {
		p, f = strings.ToLower(p), strings.ToLower(f)
	}
	return strings.HasPrefix(p, f+"/")
}

type flag int

const (
	flagUnset flag = iota
	flagFalse
	flagTrue
)

// frontmatterFlag reads the publish property. Booleans and the strings
// "true"/"false" count; anything else is ignored. If differently cased keys
// disagree, false wins.
func frontmatterFlag(fm map[string]any, key string) flag {
	if key == "" {
		return flagUnset
	}
	out := flagUnset
	for _, v := range vault.FrontmatterValue(fm, key) {
		var f flag
		switch v := v.(type) {
		case bool:
			f = flagFalse
			if v {
				f = flagTrue
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true":
				f = flagTrue
			case "false":
				f = flagFalse
			}
		}
		if f == flagFalse {
			return flagFalse
		}
		if f == flagTrue {
			out = flagTrue
		}
	}
	return out
}
