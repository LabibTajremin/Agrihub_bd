package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func gradient(w, h int, invert bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := uint8(x * 255 / w)
			if invert {
				v = 255 - v
			}
			img.Set(x, y, color.RGBA{v, v / 2, 255 - v, 255})
		}
	}
	return img
}

func encode(t *testing.T, img image.Image, asJPEG bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var err error
	if asJPEG {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDHash_SimilarImagesAreClose(t *testing.T) {
	a, err := DHash(bytes.NewReader(encode(t, gradient(200, 150, false), false)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := DHash(bytes.NewReader(encode(t, gradient(400, 300, false), true)))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := DHash(bytes.NewReader(encode(t, gradient(200, 150, true), false)))
	if d := Hamming(a, b); d > 6 {
		t.Fatalf("rescaled/recompressed copy should be near: %d", d)
	}
	if d := Hamming(a, c); d < 32 {
		t.Fatalf("mirrored image should be far: %d", d)
	}
	if _, err := DHash(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDHash_TinyImages(t *testing.T) {
	// smaller than the 9×8 grid: cells collapse but hashing stays defined
	for _, size := range [][2]int{{1, 1}, {3, 2}} {
		img := gradient(size[0], size[1], false)
		_ = dhash(img)
	}
	sub := gradient(40, 40, true).(*image.RGBA).SubImage(image.Rect(10, 10, 30, 30))
	if dhash(sub) == 0 {
		t.Fatal("offset bounds must hash a gradient")
	}
	if len(Errors()) != 6 {
		t.Fatal()
	}
}
