# svgprox

`svgprox` is a lean Go CLI that approximates a JPG/PNG image with coarse SVG polygons.

Pipeline:
1. Load + downsample image
2. Quantize colors (k-means)
3. Group pixels into connected regions per color
4. Trace region borders into polygons
5. Simplify polygon points based on complexity
6. Write SVG

## Build

Requirements: Go 1.22+

```bash
go build -o svgprox .
```

## Usage

```bash
./svgprox -in input.jpg
```

Common options:

```bash
./svgprox \
  -in input.png \
  -out output.svg \
  -colors 20 \
  -complexity 60 \
  -maxdim 360 \
  -minregion 8
```

Flags:
- `-in` (required): input image path (`.jpg`, `.jpeg`, `.png`)
- `-out`: output SVG path (default: input basename + `.svg`)
- `-colors`: palette size, `2-128` (default `16`)
- `-complexity`: polygon detail, `1-100` (higher = more points, default `50`)
- `-maxdim`: processing resolution cap for width/height (default `320`)
- `-minregion`: drop tiny regions below this pixel area (default `8`)

## Notes

- Larger `-colors` and higher `-complexity` generally increase output detail and file size.
- Higher `-maxdim` can improve shape fidelity but increases CPU and memory use.
- This is an approximation tool by design; output is intentionally coarse.

## Publish checklist

For GitHub release readiness:

1. Add sample before/after images under `examples/`.
2. Add CI (`go test ./...` + `go vet ./...`).
3. Tag releases and attach prebuilt binaries.
