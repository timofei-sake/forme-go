package templates

import (
	"encoding/json"
	"fmt"
)

// RenderTemplate evaluates compiled Forme template JSON with data and renders
// the resolved document to PDF bytes using the local WASM engine. Pass a
// json.RawMessage as data when it is already encoded as JSON.
func RenderTemplate(templateJSON []byte, data any) ([]byte, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal template data: %w", err)
	}
	return renderTemplatePDF(templateJSON, dataJSON)
}
