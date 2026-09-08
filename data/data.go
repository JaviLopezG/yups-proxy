package data

import _ "embed"

// DefaultProxiesCSV embeds the built-in proxy definitions.
//
//go:embed proxies.csv
var DefaultProxiesCSV []byte
