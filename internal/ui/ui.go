package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Renderer handles HTML template rendering.
type Renderer struct {
	indexTmpl   *template.Template
	resultsTmpl *template.Template
}

// NewRenderer parses and prepares embedded HTML templates.
func NewRenderer() (*Renderer, error) {
	indexContent, err := templatesFS.ReadFile("templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read index.html: %w", err)
	}
	indexTmpl, err := template.New("index").Parse(string(indexContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse index.html: %w", err)
	}

	resultsContent, err := templatesFS.ReadFile("templates/results.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read results.html: %w", err)
	}
	resultsTmpl, err := template.New("results").Parse(string(resultsContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse results.html: %w", err)
	}

	return &Renderer{
		indexTmpl:   indexTmpl,
		resultsTmpl: resultsTmpl,
	}, nil
}

// RenderIndex renders the Google-style minimal homepage.
func (r *Renderer) RenderIndex(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return r.indexTmpl.Execute(w, nil)
}

// ResultsViewData contains data required to render the preview/links page.
type ResultsViewData struct {
	Service            string
	Card               any
	FirstSeenFormatted string
	Proxies            []ProxyLink
}

// ProxyLink represents an available proxy destination for the results page.
type ProxyLink struct {
	Tech           string
	Description    string
	DestinationURL string
}

// RenderResults renders the proxy results & smart card page.
func (r *Renderer) RenderResults(w http.ResponseWriter, data ResultsViewData) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return r.resultsTmpl.Execute(w, data)
}

// StaticFileSystem returns an http.FileSystem serving the embedded static directory.
func StaticFileSystem() http.FileSystem {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
