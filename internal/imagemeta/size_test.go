package imagemeta

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestSize(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 40, 30))
	var p, j bytes.Buffer
	if err := png.Encode(&p, wide); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&j, wide, nil); err != nil {
		t.Fatal(err)
	}
	// The same JPEG turned a quarter by its EXIF orientation shows 30×40.
	turned := append(append([]byte{0xFF, 0xD8}, exifSegment(6)...), j.Bytes()[2:]...)

	cases := []struct {
		ext   string
		data  []byte
		w, h  int
		known bool
	}{
		{".png", p.Bytes(), 40, 30, true},
		{".JPG", j.Bytes(), 40, 30, true},
		{".jpg", turned, 30, 40, true},
		{".svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120" height="80px"/>`), 120, 80, true},
		{".svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 12"><path d="M0 0"/></svg>`), 24, 12, true},
		{".svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="96" viewBox="0,0,48,24"/>`), 96, 48, true},
		{".svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="2em"/>`), 0, 0, false},
		{".svg", []byte(`<html><svg width="1" height="1"/></html>`), 0, 0, false},
		{".avif", p.Bytes(), 0, 0, false},
		{".png", []byte("not a png"), 0, 0, false},
	}
	for i, c := range cases {
		w, h, ok := Size(c.ext, c.data)
		if w != c.w || h != c.h || ok != c.known {
			t.Errorf("%d %s: got %d×%d %v, want %d×%d %v", i, c.ext, w, h, ok, c.w, c.h, c.known)
		}
	}
}
