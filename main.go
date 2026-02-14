package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RGB struct {
	R float64
	G float64
	B float64
}

type Point struct {
	X int
	Y int
}

type Region struct {
	ColorIdx int
	Pixels   []Point
	MinX     int
	MinY     int
	MaxX     int
	MaxY     int
}

type Edge struct {
	A Point
	B Point
}

func main() {
	inPath := flag.String("in", "", "input image path (jpg/png)")
	outPath := flag.String("out", "", "output svg path (default: <input>.svg)")
	colors := flag.Int("colors", 16, "palette size (2-128)")
	complexity := flag.Int("complexity", 50, "shape detail 1-100 (higher = more points)")
	maxDim := flag.Int("maxdim", 320, "max image width/height for processing")
	minRegion := flag.Int("minregion", 8, "minimum region area in pixels to keep")
	flag.Parse()

	if *inPath == "" {
		exitErr("-in is required")
	}
	if *colors < 2 || *colors > 128 {
		exitErr("-colors must be in [2,128]")
	}
	if *complexity < 1 || *complexity > 100 {
		exitErr("-complexity must be in [1,100]")
	}
	if *maxDim < 32 {
		exitErr("-maxdim must be >= 32")
	}
	if *minRegion < 1 {
		exitErr("-minregion must be >= 1")
	}

	if *outPath == "" {
		ext := filepath.Ext(*inPath)
		base := strings.TrimSuffix(*inPath, ext)
		*outPath = base + ".svg"
	}

	img, err := loadImage(*inPath)
	if err != nil {
		exitErr(err.Error())
	}

	w0 := img.Bounds().Dx()
	h0 := img.Bounds().Dy()
	gridW, gridH := fitDims(w0, h0, *maxDim)
	pixels := sampleToGrid(img, gridW, gridH)

	palette, labels := quantizeKMeans(pixels, gridW, gridH, *colors, 8)
	regions := extractRegions(labels, gridW, gridH)
	regions = filterRegions(regions, *minRegion)
	sort.Slice(regions, func(i, j int) bool { return len(regions[i].Pixels) > len(regions[j].Pixels) })

	tol := complexityToTolerance(*complexity)
	svg, err := renderSVG(regions, labels, palette, gridW, gridH, tol)
	if err != nil {
		exitErr(err.Error())
	}

	if err := os.WriteFile(*outPath, []byte(svg), 0o644); err != nil {
		exitErr(fmt.Sprintf("write svg: %v", err))
	}

	fmt.Printf("wrote %s (%dx%d -> %dx%d, colors=%d, regions=%d)\n", *outPath, w0, h0, gridW, gridH, *colors, len(regions))
}

func exitErr(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func fitDims(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		nw := maxDim
		nh := int(math.Round(float64(h) * float64(maxDim) / float64(w)))
		if nh < 1 {
			nh = 1
		}
		return nw, nh
	}
	nh := maxDim
	nw := int(math.Round(float64(w) * float64(maxDim) / float64(h)))
	if nw < 1 {
		nw = 1
	}
	return nw, nh
}

func sampleToGrid(img image.Image, w, h int) []RGB {
	out := make([]RGB, w*h)
	b := img.Bounds()
	sx := float64(b.Dx()) / float64(w)
	sy := float64(b.Dy()) / float64(h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ix := b.Min.X + int((float64(x)+0.5)*sx)
			iy := b.Min.Y + int((float64(y)+0.5)*sy)
			if ix >= b.Max.X {
				ix = b.Max.X - 1
			}
			if iy >= b.Max.Y {
				iy = b.Max.Y - 1
			}
			r, g, bl, _ := img.At(ix, iy).RGBA()
			out[y*w+x] = RGB{
				R: float64(r >> 8),
				G: float64(g >> 8),
				B: float64(bl >> 8),
			}
		}
	}
	return out
}

func quantizeKMeans(pixels []RGB, w, h, k, iters int) ([]RGB, []int) {
	n := len(pixels)
	if k > n {
		k = n
	}
	centers := make([]RGB, k)
	for i := 0; i < k; i++ {
		centers[i] = pixels[(i*n)/k]
	}

	labels := make([]int, n)
	for iter := 0; iter < iters; iter++ {
		for i, p := range pixels {
			best := 0
			bestDist := colorDist2(p, centers[0])
			for c := 1; c < k; c++ {
				d := colorDist2(p, centers[c])
				if d < bestDist {
					bestDist = d
					best = c
				}
			}
			labels[i] = best
		}

		sums := make([]RGB, k)
		counts := make([]int, k)
		for i, p := range pixels {
			c := labels[i]
			sums[c].R += p.R
			sums[c].G += p.G
			sums[c].B += p.B
			counts[c]++
		}
		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				centers[c] = pixels[(c*n)/k]
				continue
			}
			inv := 1.0 / float64(counts[c])
			centers[c] = RGB{R: sums[c].R * inv, G: sums[c].G * inv, B: sums[c].B * inv}
		}
	}

	_ = w
	_ = h
	return centers, labels
}

func colorDist2(a, b RGB) float64 {
	dr := a.R - b.R
	dg := a.G - b.G
	db := a.B - b.B
	return dr*dr + dg*dg + db*db
}

func extractRegions(labels []int, w, h int) []Region {
	seen := make([]bool, len(labels))
	regions := make([]Region, 0, len(labels)/16)
	neighbors := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if seen[i] {
				continue
			}
			seen[i] = true
			c := labels[i]
			q := []Point{{X: x, Y: y}}
			pix := make([]Point, 0, 32)
			minX, minY, maxX, maxY := x, y, x, y

			for len(q) > 0 {
				p := q[len(q)-1]
				q = q[:len(q)-1]
				pix = append(pix, p)
				if p.X < minX {
					minX = p.X
				}
				if p.Y < minY {
					minY = p.Y
				}
				if p.X > maxX {
					maxX = p.X
				}
				if p.Y > maxY {
					maxY = p.Y
				}

				for _, d := range neighbors {
					nx, ny := p.X+d[0], p.Y+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					ni := ny*w + nx
					if seen[ni] || labels[ni] != c {
						continue
					}
					seen[ni] = true
					q = append(q, Point{X: nx, Y: ny})
				}
			}

			regions = append(regions, Region{
				ColorIdx: c,
				Pixels:   pix,
				MinX:     minX,
				MinY:     minY,
				MaxX:     maxX,
				MaxY:     maxY,
			})
		}
	}
	return regions
}

func filterRegions(regions []Region, minArea int) []Region {
	out := regions[:0]
	for _, r := range regions {
		if len(r.Pixels) >= minArea {
			out = append(out, r)
		}
	}
	return out
}

func renderSVG(regions []Region, labels []int, palette []RGB, w, h int, tol float64) (string, error) {
	if len(regions) == 0 {
		return "", errors.New("no regions left; try lowering -minregion or increasing -colors")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %d %d\" shape-rendering=\"geometricPrecision\">\n", w, h)
	fmt.Fprintf(&b, "<rect width=\"100%%\" height=\"100%%\" fill=\"%s\"/>\n", rgbHex(avgPaletteColor(palette)))

	for _, r := range regions {
		edges := regionEdges(r.Pixels, labels, w, h, r.ColorIdx)
		loops := edgeLoops(edges)
		if len(loops) == 0 {
			continue
		}
		fill := rgbHex(palette[r.ColorIdx])
		for _, loop := range loops {
			if len(loop) < 3 {
				continue
			}
			s := simplifyRDP(loop, tol)
			if len(s) < 3 {
				continue
			}
			fmt.Fprintf(&b, "<polygon fill=\"%s\" points=\"", fill)
			for i, p := range s {
				if i > 0 {
					b.WriteByte(' ')
				}
				fmt.Fprintf(&b, "%d,%d", p.X, p.Y)
			}
			b.WriteString("\"/>\n")
		}
	}
	b.WriteString("</svg>\n")
	return b.String(), nil
}

func avgPaletteColor(p []RGB) RGB {
	var s RGB
	for _, c := range p {
		s.R += c.R
		s.G += c.G
		s.B += c.B
	}
	inv := 1.0 / float64(len(p))
	return RGB{R: s.R * inv, G: s.G * inv, B: s.B * inv}
}

func rgbHex(c RGB) string {
	r := clamp255(int(math.Round(c.R)))
	g := clamp255(int(math.Round(c.G)))
	b := clamp255(int(math.Round(c.B)))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func complexityToTolerance(c int) float64 {
	// Higher complexity => lower tolerance => more polygon points.
	t := 0.25 + (100.0-float64(c))*0.04
	if t < 0.25 {
		return 0.25
	}
	if t > 4.5 {
		return 4.5
	}
	return t
}

func regionEdges(pixels []Point, labels []int, w, h, colorIdx int) []Edge {
	edges := make([]Edge, 0, len(pixels)*2)
	for _, p := range pixels {
		x, y := p.X, p.Y
		idx := y*w + x
		if labels[idx] != colorIdx {
			continue
		}

		if y == 0 || labels[(y-1)*w+x] != colorIdx {
			edges = append(edges, Edge{A: Point{X: x, Y: y}, B: Point{X: x + 1, Y: y}})
		}
		if x == w-1 || labels[y*w+x+1] != colorIdx {
			edges = append(edges, Edge{A: Point{X: x + 1, Y: y}, B: Point{X: x + 1, Y: y + 1}})
		}
		if y == h-1 || labels[(y+1)*w+x] != colorIdx {
			edges = append(edges, Edge{A: Point{X: x + 1, Y: y + 1}, B: Point{X: x, Y: y + 1}})
		}
		if x == 0 || labels[y*w+x-1] != colorIdx {
			edges = append(edges, Edge{A: Point{X: x, Y: y + 1}, B: Point{X: x, Y: y}})
		}
	}
	return edges
}

func edgeLoops(edges []Edge) [][]Point {
	next := make(map[Point][]Point, len(edges))
	for _, e := range edges {
		next[e.A] = append(next[e.A], e.B)
	}

	used := make(map[Edge]bool, len(edges))
	loops := make([][]Point, 0, len(edges)/8)

	for _, e := range edges {
		if used[e] {
			continue
		}
		start := e.A
		curr := e.B
		loop := []Point{start}
		used[e] = true

		for steps := 0; steps < len(edges)+4; steps++ {
			loop = append(loop, curr)
			if curr == start {
				break
			}
			ns := next[curr]
			if len(ns) == 0 {
				break
			}
			picked := -1
			for i, n := range ns {
				cand := Edge{A: curr, B: n}
				if !used[cand] {
					picked = i
					break
				}
			}
			if picked == -1 {
				break
			}
			nxt := ns[picked]
			used[Edge{A: curr, B: nxt}] = true
			curr = nxt
		}

		if len(loop) > 3 && loop[len(loop)-1] == start {
			loops = append(loops, loop[:len(loop)-1])
		}
	}
	return loops
}

func simplifyRDP(pts []Point, eps float64) []Point {
	if len(pts) < 3 {
		return pts
	}
	// Close loop for simplification, then reopen.
	closed := make([]Point, 0, len(pts)+1)
	closed = append(closed, pts...)
	closed = append(closed, pts[0])
	s := rdp(closed, eps)
	if len(s) > 1 && s[0] == s[len(s)-1] {
		s = s[:len(s)-1]
	}
	return s
}

func rdp(pts []Point, eps float64) []Point {
	if len(pts) <= 2 {
		return pts
	}
	start := pts[0]
	end := pts[len(pts)-1]
	maxDist := -1.0
	idx := -1
	for i := 1; i < len(pts)-1; i++ {
		d := perpDist(pts[i], start, end)
		if d > maxDist {
			maxDist = d
			idx = i
		}
	}
	if maxDist <= eps || idx < 0 {
		return []Point{start, end}
	}
	left := rdp(pts[:idx+1], eps)
	right := rdp(pts[idx:], eps)
	out := make([]Point, 0, len(left)+len(right)-1)
	out = append(out, left[:len(left)-1]...)
	out = append(out, right...)
	return out
}

func perpDist(p, a, b Point) float64 {
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	px, py := float64(p.X), float64(p.Y)

	dx := bx - ax
	dy := by - ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	num := math.Abs(dy*px - dx*py + bx*ay - by*ax)
	den := math.Hypot(dx, dy)
	return num / den
}
