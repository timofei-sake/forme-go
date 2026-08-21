//go:build forme_wasm

// These tests drive the embedded forme.wasm through the public API. Unlike the
// WASM tests in templates_test.go they never skip: under the forme_wasm tag the
// engine is compiled into the binary, so an engine that cannot render is a
// failure and not a reason to pass quietly.
package templates

import (
	"strings"
	"testing"
)

// renderIterations is deliberately more than one. RenderTemplate instantiates a
// fresh module per call against one compiled module and one runtime, so a
// module whose static state leaks across instantiations - or a host-side buffer
// that is freed while still referenced - shows up on the second call and later,
// not the first.
const renderIterations = 5

// invoiceTemplate binds every value it prints, including text outside Latin-1
// so that the render has to go through the builtin Noto Sans fonts rather than
// the standard PDF font tables.
const invoiceTemplate = `{
	"children": [
		{
			"kind": {"type": "Text", "content": {"$ref": "title"}},
			"style": {"fontSize": 24},
			"children": []
		},
		{
			"kind": {"type": "Text", "content": {"$ref": "customer.name"}},
			"style": {"fontSize": 12, "fontFamily": "Noto Sans"},
			"children": []
		},
		{
			"kind": {"type": "Text", "content": {"$ref": "customer.city"}},
			"style": {"fontSize": 12, "fontFamily": "Noto Sans"},
			"children": []
		}
	],
	"metadata": {"title": {"$ref": "title"}},
	"defaultPage": {
		"size": "A4",
		"margin": {"top": 54, "right": 54, "bottom": 54, "left": 54},
		"wrap": true
	}
}`

func invoiceData() map[string]any {
	return map[string]any{
		"title": "Invoice #001",
		"customer": map[string]any{
			"name": "Ada Lovelace",
			"city": "Ки́ев — Λονδίνο",
		},
	}
}

// head bounds what a failure message quotes back from a result that is not a
// PDF, so a megabyte of binary never lands in the test log.
func head(out []byte) []byte {
	if len(out) > 16 {
		return out[:16]
	}

	return out
}

// TestRenderTemplate_RendersRepeatedly is the end-to-end contract of the
// package: compiled template JSON plus data in, a PDF out, every time.
func TestRenderTemplate_RendersRepeatedly(t *testing.T) {
	var first int

	for i := 0; i < renderIterations; i++ {
		pdf, err := RenderTemplate([]byte(invoiceTemplate), invoiceData())
		if err != nil {
			t.Fatalf("iteration %d: RenderTemplate: %v", i, err)
		}
		if len(pdf) == 0 {
			t.Fatalf("iteration %d: RenderTemplate returned no bytes", i)
		}
		if !strings.HasPrefix(string(pdf), "%PDF-") {
			t.Fatalf("iteration %d: result is not a PDF: %q", i, head(pdf))
		}

		// The same template and the same data must produce the same amount of
		// output. A render that silently drops content - an unresolved
		// reference, a font that failed to register - shows up as a shorter
		// PDF while still starting with %PDF-.
		if i == 0 {
			first = len(pdf)

			continue
		}
		if len(pdf) != first {
			t.Fatalf("iteration %d: rendered %d bytes, the first render produced %d", i, len(pdf), first)
		}
	}
}

// A reference the data does not satisfy must be reported. This is the failure
// the caller has to be able to see, and it is what tells a template compiled
// against one data shape from one compiled against another.
func TestRenderTemplate_ReportsAnUnresolvedReference(t *testing.T) {
	_, err := RenderTemplate([]byte(invoiceTemplate), map[string]any{"title": "Invoice #001"})
	if err == nil {
		t.Fatal("expected an error for a template referencing absent data")
	}
}

// Malformed template JSON is rejected by the engine, not by a host-side parse,
// so this pins that the error crosses the ABI boundary as an error rather than
// as a trap.
func TestRenderTemplate_RejectsMalformedTemplate(t *testing.T) {
	_, err := RenderTemplate([]byte(`{"children":`), invoiceData())
	if err == nil {
		t.Fatal("expected an error for malformed template JSON")
	}
}
