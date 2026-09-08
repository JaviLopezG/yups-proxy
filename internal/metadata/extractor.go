package metadata

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CardData contains OpenGraph / Twitter Card preview information.
type CardData struct {
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Image       string    `json:"image"`
	SiteName    string    `json:"site_name"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// ExtractFromHTML parses an HTML document up to 512KB and extracts metadata.
func ExtractFromHTML(body io.Reader, rawURL string) (*CardData, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	card := &CardData{
		URL:         rawURL,
		SiteName:    parsedURL.Hostname(),
		FetchedAt:   time.Now().UTC(),
		FirstSeenAt: time.Now().UTC(),
	}

	tokenizer := html.NewTokenizer(body)

	var inTitle bool
	var titleText string

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			// EOF or parsing error: finalize
			finalizeCard(card, titleText, parsedURL)
			return card, nil

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)

			if tagName == "title" {
				inTitle = true
				continue
			}

			if tagName == "meta" {
				var prop, name, content string
				for _, attr := range token.Attr {
					key := strings.ToLower(strings.TrimSpace(attr.Key))
					val := strings.TrimSpace(attr.Val)
					switch key {
					case "property":
						prop = strings.ToLower(val)
					case "name":
						name = strings.ToLower(val)
					case "content":
						content = val
					}
				}

				if content == "" {
					continue
				}

				switch prop {
				case "og:title":
					if card.Title == "" {
						card.Title = content
					}
				case "og:description":
					if card.Description == "" {
						card.Description = content
					}
				case "og:image", "og:image:url", "og:image:secure_url":
					if card.Image == "" {
						card.Image = resolveURL(parsedURL, content)
					}
				case "og:site_name":
					card.SiteName = content
				}

				switch name {
				case "twitter:title":
					if card.Title == "" {
						card.Title = content
					}
				case "twitter:description":
					if card.Description == "" {
						card.Description = content
					}
				case "twitter:image", "twitter:image:src":
					if card.Image == "" {
						card.Image = resolveURL(parsedURL, content)
					}
				case "description":
					if card.Description == "" {
						card.Description = content
					}
				}
			}

			// If we entered <body>, stop to save time
			if tagName == "body" {
				finalizeCard(card, titleText, parsedURL)
				return card, nil
			}

		case html.EndTagToken:
			token := tokenizer.Token()
			if strings.ToLower(token.Data) == "title" {
				inTitle = false
			}

		case html.TextToken:
			if inTitle {
				titleText += string(tokenizer.Text())
			}
		}
	}
}

func finalizeCard(card *CardData, fallbackTitle string, parsedURL *url.URL) {
	if card.Title == "" {
		card.Title = strings.TrimSpace(fallbackTitle)
	}
	if card.Title == "" {
		card.Title = parsedURL.Hostname()
	}
	if card.Description == "" {
		card.Description = fmt.Sprintf("View %s content via YUPS proxy service.", parsedURL.Hostname())
	}
	if card.Image == "" {
		card.Image = "/static/logo.png"
	}
}

func resolveURL(base *url.URL, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	if parsed.IsAbs() {
		return target
	}
	return base.ResolveReference(parsed).String()
}
