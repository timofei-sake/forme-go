//go:build forme_wasm

// These tests pin the draw order of the embedded engine. Everything asserted
// here is measured out of the page content stream: an image the engine
// embedded but never painted still leaves a valid PDF, a plausible file size
// and no error, so nothing short of reading what the page draws can tell the
// two apart.
package templates

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"testing"
)

// Two PNGs with different pixel dimensions. Distinct sizes are what makes a
// shifted XObject lookup visible: a chart that consumed an image's slot in the
// engine's draw-order counter used to paint the wrong image before it ran off
// the end of the map and painted none.
const (
	redPNG4  = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAEElEQVR42mP4z8AARwzEcQCukw/xOF6MEQAAAABJRU5ErkJggg=="
	bluePNG8 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAgAAAAICAIAAABLbSncAAAAEElEQVR42mNgYPiPAw0pCQCpcD/B/MtF/AAAAABJRU5ErkJggg=="
)

const barChartNode = `{
	"kind": {
		"type": "BarChart",
		"data": [{"label": "Q1", "value": 100}, {"label": "Q2", "value": 150}],
		"width": 200,
		"height": 120,
		"show_labels": true
	},
	"style": {},
	"children": []
}`

func imageNode(src string, size float64) string {
	points := strconv.FormatFloat(size, 'f', -1, 64)

	return `{
		"kind": {"type": "Image", "src": "` + src + `", "width": ` + points + `, "height": ` + points + `},
		"style": {},
		"children": []
	}`
}

// drawOrderDocument wraps nodes into a template the engine will accept. The
// template carries no `$ref`, so RenderTemplate's data argument is unused.
func drawOrderDocument(nodes ...string) []byte {
	body := ""
	for i, node := range nodes {
		if i > 0 {
			body += ","
		}

		body += node
	}

	return []byte(`{
		"children": [` + body + `],
		"metadata": {},
		"defaultPage": {"size": "A4", "margin": {"top": 36, "right": 36, "bottom": 36, "left": 36}},
		"fonts": []
	}`)
}

// placedImagePattern matches the operator pair the engine emits for every
// image it actually paints: "w 0 0 h x y cm /ImN Do".
var placedImagePattern = regexp.MustCompile(
	`([\d.-]+) 0 0 ([\d.-]+) ([\d.-]+) ([\d.-]+) cm\s*/(\w+) Do`)

var streamPattern = regexp.MustCompile(`stream\r?\n`)

// pageContent returns the page content streams, decompressed. Image streams
// are skipped rather than concatenated: decompressed pixel data is arbitrary
// bytes and reads as a drawing operator often enough to matter.
func pageContent(t *testing.T, pdf []byte) string {
	t.Helper()

	var content bytes.Buffer

	for _, marker := range streamPattern.FindAllIndex(pdf, -1) {
		end := bytes.Index(pdf[marker[1]:], []byte("endstream"))
		if end < 0 || isImageObject(pdf[:marker[0]]) {
			continue
		}

		reader, err := zlib.NewReader(bytes.NewReader(pdf[marker[1] : marker[1]+end]))
		if err != nil {
			continue
		}

		decompressed, err := io.ReadAll(reader)
		_ = reader.Close()

		if err == nil {
			content.Write(decompressed)
		}
	}

	if content.Len() == 0 {
		t.Fatal("no page content stream could be read")
	}

	return content.String()
}

// isImageObject reports whether the object whose dictionary ends at the end of
// preamble is an image XObject.
func isImageObject(preamble []byte) bool {
	start := bytes.LastIndex(preamble, []byte(" obj"))
	if start < 0 {
		return false
	}

	return bytes.Contains(preamble[start:], []byte("/Subtype /Image"))
}

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}

	return value
}

type placedImage struct {
	name  string
	width float64
}

// drawnImages returns the images the document paints, which is not the same as
// the images it carries: an embedded XObject nothing draws is invisible.
func drawnImages(t *testing.T, pdf []byte) []placedImage {
	t.Helper()

	matches := placedImagePattern.FindAllStringSubmatch(pageContent(t, pdf), -1)
	placed := make([]placedImage, 0, len(matches))

	for _, match := range matches {
		placed = append(placed, placedImage{name: match[5], width: parseFloat(t, match[1])})
	}

	return placed
}

// resolvesToImageXObject reports whether name is bound in a page /XObject resource dict
// to an object that really is an image XObject. A painted /ImN that resolves
// to nothing is a broken reference, not a placement.
func resolvesToImageXObject(t *testing.T, pdf []byte, name string) bool {
	t.Helper()

	entry := regexp.MustCompile(`/` + regexp.QuoteMeta(name) + ` (\d+) 0 R`).FindSubmatch(pdf)
	if entry == nil {
		return false
	}

	header := append([]byte("\n"), append(entry[1], []byte(" 0 obj\n")...)...)

	start := bytes.Index(pdf, header)
	if start < 0 {
		return false
	}

	head := pdf[start+len(header):]
	if len(head) > 512 {
		head = head[:512]
	}

	return bytes.Contains(head, []byte("/Subtype /Image"))
}

// assertImagesPainted checks that the document paints exactly the given image
// widths, in order, and that every painted name resolves to a real image.
func assertImagesPainted(t *testing.T, pdf []byte, widths ...float64) {
	t.Helper()

	painted := drawnImages(t, pdf)
	if len(painted) != len(widths) {
		t.Fatalf("expected %d image(s) painted, got %d: %+v", len(widths), len(painted), painted)
	}

	for i, image := range painted {
		if image.width != widths[i] {
			t.Errorf("image %d painted at width %v, want %v (placements: %+v)", i, image.width, widths[i], painted)
		}

		if !resolvesToImageXObject(t, pdf, image.name) {
			t.Errorf("/%s is painted but does not resolve to an image XObject", image.name)
		}
	}
}

func renderDrawOrder(t *testing.T, nodes ...string) []byte {
	t.Helper()

	pdf, err := RenderTemplate(drawOrderDocument(nodes...), map[string]any{})
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}

	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("result is not a PDF: %q", head(pdf))
	}

	return pdf
}

// TestImageBeforeChartIsPainted is the control: this ordering always worked,
// and it has to keep working.
func TestImageBeforeChartIsPainted(t *testing.T) {
	assertImagesPainted(t, renderDrawOrder(t, imageNode(redPNG4, 60), barChartNode), 60)
}

// TestChartBeforeImageIsPainted is the regression. A chart drawn ahead of an
// image on the same page used to leave the image embedded but never painted.
func TestChartBeforeImageIsPainted(t *testing.T) {
	assertImagesPainted(t, renderDrawOrder(t, barChartNode, imageNode(redPNG4, 60)), 60)
}

// TestTwoChartsBeforeTwoImagesPaintBoth pins the shape of the old failure: N
// charts ahead of the images swallowed the last N of them, and the images that
// did survive were painted with the wrong XObject.
func TestTwoChartsBeforeTwoImagesPaintBoth(t *testing.T) {
	pdf := renderDrawOrder(t,
		barChartNode, barChartNode,
		imageNode(redPNG4, 60), imageNode(bluePNG8, 90),
	)
	assertImagesPainted(t, pdf, 60, 90)
}

// TestChartsAndImagesInterleavedPaintEveryImage covers the ordering a report
// page actually produces: a chart, its figure, the next chart, its figure.
func TestChartsAndImagesInterleavedPaintEveryImage(t *testing.T) {
	pdf := renderDrawOrder(t,
		barChartNode, imageNode(redPNG4, 60),
		barChartNode, imageNode(bluePNG8, 90),
	)
	assertImagesPainted(t, pdf, 60, 90)
}
