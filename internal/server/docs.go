package server

import (
	_ "embed"
	"net/http"
)

// openAPISpec is the generated OpenAPI v3 document, copied into this package by
// `task gen` (see taskfiles/proto.yml).
//
//go:embed openapi.yaml
var openAPISpec []byte

// openAPIHandler serves the raw OpenAPI spec.
func openAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openAPISpec)
	}
}

// docsHandler serves a Scalar-rendered API reference pointing at the spec.
func docsHandler() http.HandlerFunc {
	const page = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>ZKit API</title>
  </head>
  <body>
    <script id="api-reference" data-url="/api/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}
}
