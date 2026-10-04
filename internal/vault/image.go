package vault

import "strings"

// Image properties name a picture in the frontmatter: a note's social
// preview image, or the site's default image and avatar in the settings
// note. Both gates publish the vault image such a property points to, as if
// the note embedded it, so the property works without also embedding the
// picture in the text. The plugin mirrors this in rules.ts.
//
// Each list is one purpose; the first of its keys that holds an image wins.
var (
	NoteImageKeys  = []string{"image", "cover"} // a note's social preview image
	SiteImageKeys  = []string{"image"}          // the site's default social preview image
	SiteAvatarKeys = []string{"avatar", "logo"} // the profile picture
)

// ImageRef is the value of an image property: a link to a vault file, or a
// web address.
type ImageRef struct {
	Link Link   // set for vault files
	URL  string // set for http(s) addresses
}

// ImageProperty reads the first of keys (case-insensitive) that holds an
// image. A value may be a link ("[[cover.png]]", "![[cover.png]]"), a vault
// path or file name ("attachments/cover.png") or an http(s) URL; for a list
// the first item counts.
func ImageProperty(fm map[string]any, keys []string) (ImageRef, bool) {
	for _, key := range keys {
		for _, v := range FrontmatterValue(fm, key) {
			if r, ok := ParseImageValue(v); ok {
				return r, true
			}
		}
	}
	return ImageRef{}, false
}

// ParseImageValue interprets one image property value.
func ParseImageValue(v any) (ImageRef, bool) {
	if list, ok := v.([]any); ok {
		if len(list) == 0 {
			return ImageRef{}, false
		}
		v = list[0]
	}
	s, ok := v.(string)
	if !ok {
		return ImageRef{}, false
	}
	s = strings.TrimPrefix(strings.TrimSpace(s), "!")
	if inner, ok := strings.CutPrefix(s, "[["); ok {
		inner, ok = strings.CutSuffix(inner, "]]")
		if !ok {
			return ImageRef{}, false
		}
		l := parseWikilink(inner)
		if l.Target == "" {
			return ImageRef{}, false
		}
		l.Embed = true
		return ImageRef{Link: l}, true
	}
	if isExternal(s) {
		if ls := strings.ToLower(s); strings.HasPrefix(ls, "https://") || strings.HasPrefix(ls, "http://") {
			return ImageRef{URL: s}, true
		}
		return ImageRef{}, false
	}
	if s == "" || strings.ContainsAny(s, "[]\n") {
		return ImageRef{}, false
	}
	target, _, _ := strings.Cut(s, "#")
	return ImageRef{Link: Link{Target: strings.TrimSpace(target), Embed: true}}, true
}

// IsImage reports whether a vault path has an image extension.
func IsImage(p string) bool {
	name := p[strings.LastIndexByte(p, '/')+1:]
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return false // no extension, or a dot file
	}
	switch strings.ToLower(name[i+1:]) {
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "avif", "bmp":
		return true
	}
	return false
}
