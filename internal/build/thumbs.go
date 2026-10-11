package build

import (
	"errors"
	"io"
	"path"
	"strings"

	"github.com/klppl/kvist/internal/model"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/source"
	"github.com/klppl/kvist/internal/thumb"
)

// thumbSizes are the small copies of a note's image (Note.ImageThumb,
// Note.ImageCard), in pixels: about twice what the garden theme shows, so
// they stay sharp on high-density screens.
var thumbSizes = [2]struct {
	name string
	w, h int
}{{"thumb", 128, 128}, {"card", 640, 360}}

// thumbVersion changes every thumbnail's URL when the drawing changes.
const thumbVersion = "1"

// thumbnails makes the small copies of each note's image and points
// Note.ImageThumb and Note.ImageCard at them. It runs before rendering, so
// themes see the final URLs. Thumbnails are named by the source image's
// hash, so an unchanged one is reused from the previous build without
// decoding the image again. An image that can't be read keeps the full
// image in both fields.
func thumbnails(m *model.Site, snap source.Snapshot, files map[string]source.File, out *dirOutput) ([]protocol.Warning, error) {
	assets := map[string]*model.Asset{}
	for _, a := range m.Assets {
		assets[a.URL] = a
	}
	var warnings []protocol.Warning
	done := map[string]*[2]string{} // by image URL; nil: keep the image
	for _, n := range m.Notes {
		a := assets[n.Image]
		if a == nil || !thumb.Supported(path.Ext(a.Path)) {
			continue // an SVG, a web address
		}
		urls, ok := done[n.Image]
		if !ok {
			f, ok := files[a.Path]
			if !ok {
				continue
			}
			var err error
			urls, err = makeThumbs(a, f, snap, out)
			if err == errUnreadable {
				warnings = append(warnings, protocol.Warning{Code: protocol.WarnBuild, Path: a.Path,
					Message: "image could not be read to make a small copy for note lists; they show it full size"})
			} else if err != nil {
				return nil, err
			}
			done[n.Image] = urls
		}
		if urls != nil {
			n.ImageThumb, n.ImageCard = urls[0], urls[1]
		}
	}
	return warnings, nil
}

var errUnreadable = errors.New("image could not be decoded")

// makeThumbs writes (or reuses) the thumbnails of one image. It returns
// errUnreadable, and no URLs, for an image it can't decode.
func makeThumbs(a *model.Asset, f source.File, snap source.Snapshot, out *dirOutput) (*[2]string, error) {
	stem := strings.TrimSuffix(path.Base(a.URL), path.Ext(a.URL))
	base := func(i int) string {
		s := thumbSizes[i]
		h := strings.TrimPrefix(protocol.HashBytes([]byte(f.Hash+":thumb"+thumbVersion+":"+s.name)), protocol.HashPrefix)
		return "_assets/" + h[:8] + "/" + stem + "-" + s.name + "."
	}
	var urls [2]string
	missing := false
	for i := range thumbSizes {
		for _, ext := range []string{"jpg", "png"} {
			if out.reuse(base(i) + ext) {
				urls[i] = "/" + base(i) + ext
				break
			}
		}
		missing = missing || urls[i] == ""
	}
	if !missing {
		return &urls, nil
	}
	r, err := snap.Open(f)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		return nil, err
	}
	im, err := thumb.Decode(path.Ext(a.Path), data)
	if err != nil {
		return nil, errUnreadable
	}
	for i, s := range thumbSizes {
		if urls[i] != "" {
			continue
		}
		b, ext, err := im.Make(s.w, s.h)
		if err != nil {
			return nil, errUnreadable
		}
		if err := out.WriteFile(base(i)+ext, b); err != nil {
			return nil, err
		}
		urls[i] = "/" + base(i) + ext
	}
	return &urls, nil
}
