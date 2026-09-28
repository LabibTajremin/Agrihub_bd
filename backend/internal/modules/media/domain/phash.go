package domain

import (
	"image"
	_ "image/jpeg" // register decoders for DHash
	_ "image/png"
	"io"
	"math/bits"
)

// DHash computes a 64-bit difference hash: the image is reduced to a 9×8
// grayscale grid by box averaging and each bit records whether a cell is
// brighter than its right neighbour. Near-identical photos of the same leaf
// differ in only a few bits (see Hamming), so the hash keys the scan cache.
func DHash(r io.Reader) (uint64, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return 0, err
	}
	return dhash(img), nil
}

func dhash(img image.Image) uint64 {
	const w, h = 9, 8
	b := img.Bounds()
	var grid [h][w]uint64
	for gy := range h {
		y0 := b.Min.Y + gy*b.Dy()/h
		y1 := max(b.Min.Y+(gy+1)*b.Dy()/h, y0+1)
		for gx := range w {
			x0 := b.Min.X + gx*b.Dx()/w
			x1 := max(b.Min.X+(gx+1)*b.Dx()/w, x0+1)
			var sum, n uint64
			for y := y0; y < y1 && y < b.Max.Y; y++ {
				for x := x0; x < x1 && x < b.Max.X; x++ {
					cr, cg, cb, _ := img.At(x, y).RGBA()
					sum += (299*uint64(cr) + 587*uint64(cg) + 114*uint64(cb)) / 1000
					n++
				}
			}
			grid[gy][gx] = sum / n // every cell spans at least one pixel
		}
	}
	var hash uint64
	for y := range h {
		for x := range w - 1 {
			hash <<= 1
			if grid[y][x] > grid[y][x+1] {
				hash |= 1
			}
		}
	}
	return hash
}

// Hamming is the number of differing bits between two hashes.
func Hamming(a, b uint64) int { return bits.OnesCount64(a ^ b) }
