package model

// AutoConnection is private server-side connection information. The token must
// never be included in browser-facing responses.
type AutoConnection struct {
	URL   string `json:"url"`
	Token string `json:"-"`
}
