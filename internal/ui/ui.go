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
	helpTmpl    *template.Template
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

	helpContent, err := templatesFS.ReadFile("templates/help.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read help.html: %w", err)
	}
	helpTmpl, err := template.New("help").Parse(string(helpContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse help.html: %w", err)
	}

	return &Renderer{
		indexTmpl:   indexTmpl,
		resultsTmpl: resultsTmpl,
		helpTmpl:    helpTmpl,
	}, nil
}

// IndexViewData contains data required to render the homepage.
type IndexViewData struct {
	PrefixURL string
}

// RenderIndex renders the Google-style minimal homepage.
func (r *Renderer) RenderIndex(w http.ResponseWriter, data IndexViewData) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return r.indexTmpl.Execute(w, data)
}

// HelpViewData contains data required to render the help / step-by-step page.
type HelpViewData struct {
	ErrorMessage string
	BaseURL      string
}

// RenderHelp renders the step-by-step help and guide page.
func (r *Renderer) RenderHelp(w http.ResponseWriter, data HelpViewData, statusCode int) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if statusCode > 0 {
		w.WriteHeader(statusCode)
	}
	return r.helpTmpl.Execute(w, data)
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
