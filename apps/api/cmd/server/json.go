package main

import (
	"encoding/json"
	"io"
)

// encodeJSON writes body as JSON to w.
func encodeJSON(w io.Writer, body any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(body)
}
