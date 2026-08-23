// Command genicons draws the QueueUp app icons: an orange rounded square with
// a rising three-bar "queue" mark. Run once, outputs committed:
//
//	go run ./tools/genicons
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

var (
	bg     = color.NRGBA{0x1a, 0x1d, 0x21, 0xff} // page dark
	orange = color.NRGBA{0xd0, 0x5a, 0x2a, 0xff}
	cream  = color.NRGBA{0xee, 0xf1, 0xf4, 0xff}
)

func main() {
	for _, spec := range []struct {
		path string
		size int
	}{
		{"web/public/icon-192.png", 192},
		{"web/public/icon-512.png", 512},
		{"web/public/apple-touch-icon.png", 180},
	} {
		if err := write(spec.path, spec.size); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", spec.path)
	}
}

func write(path string, size int) error {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	radius := s * 0.22

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, pixel(float64(x)+0.5, float64(y)+0.5, s, radius))
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// pixel decides the colour at one point: dark rounded square, three orange
// bars rising left to right (a queue moving forward), the last one cream (in).
func pixel(x, y, s, r float64) color.NRGBA {
	if !inRoundedSquare(x, y, s, r) {
		return color.NRGBA{} // transparent corner
	}
	// Three bars on a baseline at 76% height.
	base := s * 0.76
	barW := s * 0.14
	gap := s * 0.10
	left := (s - (3*barW + 2*gap)) / 2
	heights := []float64{0.18, 0.30, 0.44}
	colors := []color.NRGBA{orange, orange, cream}
	for i := 0; i < 3; i++ {
		x0 := left + float64(i)*(barW+gap)
		top := base - heights[i]*s
		if x >= x0 && x <= x0+barW && y >= top && y <= base {
			return colors[i]
		}
	}
	return bg
}

func inRoundedSquare(x, y, s, r float64) bool {
	if x < 0 || y < 0 || x > s || y > s {
		return false
	}
	// Inside the middle cross is always in; corners need the radius check.
	cx := clamp(x, r, s-r)
	cy := clamp(y, r, s-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
