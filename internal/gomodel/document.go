package gomodel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const maxParsedDocumentBytes = 8 << 20

var documentModelID = regexp.MustCompile(`^cline-pass/[a-z0-9][a-z0-9._-]{0,127}$`)

// ParseDocument reads the supported-model table under the documentation's Models
// heading. It deliberately ignores model IDs in examples, notices and pricing.
// An invalid row rejects the whole update so a partial document cannot silently
// remove models from the last successful catalog.
func ParseDocument(r io.Reader) ([]Info, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxParsedDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read model document: %w", err)
	}
	if len(data) > maxParsedDocumentBytes {
		return nil, errors.New("model document exceeds size limit")
	}
	if err := documentTablesComplete(data); err != nil {
		return nil, err
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse model document: %w", err)
	}
	var elements []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "pre", "template", "nav", "aside", "header", "footer":
				return
			}
			if documentHeadingLevel(n) != 0 || n.Data == "table" {
				elements = append(elements, n)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)

	var models []Info
	sectionLevel := 0
	foundSection := false
	sectionHasTable := false
	for _, n := range elements {
		if level := documentHeadingLevel(n); level != 0 {
			isModels := strings.EqualFold(documentAttr(n, "id"), "models") || strings.EqualFold(documentText(n), "Models")
			if sectionLevel != 0 && (level <= sectionLevel || isModels) {
				if !sectionHasTable {
					return nil, errors.New("Models section has no model table")
				}
				sectionLevel = 0
			}
			if isModels {
				foundSection = true
				sectionLevel = level
				sectionHasTable = false
			}
			continue
		}
		if sectionLevel == 0 {
			continue
		}
		parsed, matched, err := parseDocumentTable(n)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		sectionHasTable = true
		if models == nil {
			models = parsed
		} else if !sameDocumentModels(models, parsed) {
			return nil, errors.New("model document contains conflicting model tables")
		}
	}
	if !foundSection {
		return nil, errors.New("model document has no Models heading")
	}
	if models == nil || (sectionLevel != 0 && !sectionHasTable) {
		return nil, errors.New("Models section has no model table")
	}
	return models, nil
}

// The HTML tree builder repairs truncated tables. Check their explicit closing
// tags first so an interrupted response is not accepted as a smaller catalog.
func documentTablesComplete(data []byte) error {
	z := html.NewTokenizer(bytes.NewReader(data))
	depth := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return fmt.Errorf("read model HTML: %w", err)
			}
			if depth != 0 {
				return errors.New("model document contains an incomplete table")
			}
			return nil
		case html.StartTagToken:
			name, _ := z.TagName()
			if string(name) == "table" {
				depth++
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "table" {
				depth--
				if depth < 0 {
					return errors.New("model document contains malformed table markup")
				}
			}
		}
	}
}

func parseDocumentTable(table *html.Node) ([]Info, bool, error) {
	var rows []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n != table && n.Type == html.ElementNode && n.Data == "table" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			rows = append(rows, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(table)
	if len(rows) == 0 {
		return nil, false, nil
	}
	headers := documentCells(rows[0])
	if len(headers) != 2 {
		return nil, false, nil
	}
	nameColumn, idColumn := -1, -1
	for i, header := range headers {
		switch strings.ToLower(documentText(header)) {
		case "model":
			nameColumn = i
		case "model id":
			idColumn = i
		}
	}
	if nameColumn < 0 || idColumn < 0 {
		return nil, false, nil
	}
	if len(rows) == 1 {
		return nil, true, errors.New("model table is empty")
	}
	models := make([]Info, 0, len(rows)-1)
	seen := make(map[string]string)
	for i, row := range rows[1:] {
		cells := documentCells(row)
		if len(cells) != 2 {
			return nil, true, fmt.Errorf("model table row %d must contain two cells", i+1)
		}
		for _, cell := range cells {
			if span := documentAttr(cell, "colspan"); span != "" && span != "1" {
				return nil, true, fmt.Errorf("model table row %d contains merged cells", i+1)
			}
			if span := documentAttr(cell, "rowspan"); span != "" && span != "1" {
				return nil, true, fmt.Errorf("model table row %d contains merged cells", i+1)
			}
		}
		name := documentText(cells[nameColumn])
		id, count := documentCode(cells[idColumn])
		if name == "" || len(name) > 256 || count != 1 || !documentModelID.MatchString(id) || documentText(cells[idColumn]) != id {
			return nil, true, fmt.Errorf("model table row %d contains an invalid name or model ID", i+1)
		}
		if previous, ok := seen[id]; ok {
			if previous != name {
				return nil, true, fmt.Errorf("model table row %d conflicts with a duplicate model ID", i+1)
			}
			continue
		}
		seen[id] = name
		models = append(models, Info{ID: id, Name: name, Endpoint: EndpointChat})
	}
	return models, true, nil
}

func documentCells(row *html.Node) []*html.Node {
	var cells []*html.Node
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			cells = append(cells, c)
		}
	}
	return cells
}

func documentHeadingLevel(n *html.Node) int {
	if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
		return int(n.Data[1] - '0')
	}
	return 0
}

func documentAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func documentText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "svg", "template":
				return
			case "br":
				b.WriteByte(' ')
			}
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return strings.Join(strings.Fields(strings.NewReplacer("\u200b", "", "\ufeff", "").Replace(b.String())), " ")
}

func documentCode(n *html.Node) (string, int) {
	var value string
	count := 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "code" {
			value = documentText(node)
			count++
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return value, count
}

func sameDocumentModels(a, b []Info) bool {
	if len(a) != len(b) {
		return false
	}
	byID := make(map[string]string, len(a))
	for _, model := range a {
		byID[model.ID] = model.Name
	}
	for _, model := range b {
		if byID[model.ID] != model.Name {
			return false
		}
	}
	return true
}
