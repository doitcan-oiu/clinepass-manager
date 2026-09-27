package gomodel

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const documentTableStart = `<table><thead><tr><th>Model</th><th>Model ID</th></tr></thead><tbody>`

func documentTestTable(rows string) string {
	return documentTableStart + rows + `</tbody></table>`
}

func documentTestRow(name, id string) string {
	return `<tr><td>` + name + `</td><td><code>` + id + `</code></td></tr>`
}

func TestParseDocumentOfficialModelsSection(t *testing.T) {
	want := []Info{
		{ID: "cline-pass/glm-5.3", Name: "GLM-5.3", Endpoint: EndpointChat},
		{ID: "cline-pass/glm-5.3-flash", Name: "GLM-5.3 Flash", Endpoint: EndpointChat},
		{ID: "cline-pass/kimi-k3", Name: "Kimi K3", Endpoint: EndpointChat},
		{ID: "cline-pass/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Endpoint: EndpointChat},
		{ID: "cline-pass/deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Endpoint: EndpointChat},
		{ID: "cline-pass/mimo-v2.5", Name: "MiMo-V2.5", Endpoint: EndpointChat},
		{ID: "cline-pass/mimo-v2.5-pro", Name: "MiMo-V2.5-Pro", Endpoint: EndpointChat},
		{ID: "cline-pass/minimax-m3", Name: "MiniMax M3", Endpoint: EndpointChat},
		{ID: "cline-pass/muse-spark-1.3-contributor", Name: "Muse Spark 1.3 Contributor", Endpoint: EndpointChat},
		{ID: "cline-pass/qwen3.8-max", Name: "Qwen3.8 Max", Endpoint: EndpointChat},
		{ID: "cline-pass/qwen3.7-max", Name: "Qwen3.7 Max", Endpoint: EndpointChat},
		{ID: "cline-pass/qwen3.7-plus", Name: "Qwen3.7 Plus", Endpoint: EndpointChat},
	}
	var rows strings.Builder
	for _, model := range want {
		rows.WriteString(documentTestRow(model.Name, model.ID))
	}
	doc := `<html><body><nav><a href="#models">Models</a></nav><main>
	<h2 id="old-models">Old models</h2>` + documentTestTable(documentTestRow("Old", "cline-pass/old")) + `
	<h2 id="models"><div><a href="#models" aria-label="Navigate to header">​<svg><title>Link</title></svg></a></div><span>Models</span></h2>
	<p>ClinePass includes the following models, tested and benchmarked for coding agent use:</p>
	<div data-component-part="scroll-area"><div data-component-part="scroll-area-viewport"><div class="table">` + documentTestTable(rows.String()) + `</div></div></div>
	<div role="note"><strong>Model deprecations:</strong> <code>cline-pass/glm-5.2</code>, <code>cline-pass/kimi-k2.6</code></div>
	<h2 id="using-clinepass-outside-of-cline">Using ClinePass outside of Cline</h2>
	<pre><code>{"model":"cline-pass/example-only"}</code></pre>
	<h2 id="reference-pricing">Reference pricing</h2>
	<table><tr><th>Model</th><th>Input</th></tr><tr><td><code>cline-pass/price-only</code></td><td>$1</td></tr></table>
	` + documentTestTable(documentTestRow("Unrelated", "cline-pass/unrelated")) + `
	</main><script>const model = "cline-pass/script-only";</script></body></html>`
	got, err := ParseDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestParseDocumentAdaptsToMarkupAndCatalogChanges(t *testing.T) {
	doc := `<article><section id="models"><div><h3><a>​</a><strong>Models</strong></h3></div>
	<div><section><table><tr><th><strong>Model&nbsp; ID</strong></th><th>Model</th></tr>
	<tr><td><strong><code>cline-pass/brand-new-v7</code></strong></td><td><a>Brand <em>New</em>&nbsp;V7</a></td></tr>
	<tr><td><code>cline-pass/another-v8</code></td><td>Another<br>V8</td></tr>
	</table></section></div></section><h3>Examples</h3>
	` + documentTestTable(documentTestRow("Excluded", "cline-pass/excluded")) + `</article>`
	got, err := ParseDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := []Info{
		{ID: "cline-pass/brand-new-v7", Name: "Brand New V7", Endpoint: EndpointChat},
		{ID: "cline-pass/another-v8", Name: "Another V8", Endpoint: EndpointChat},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestParseDocumentDeduplicatesIdenticalResponsiveTables(t *testing.T) {
	row := documentTestRow("Model A", "cline-pass/a")
	table := documentTestTable(row + row)
	for _, doc := range []string{
		`<h2 id="models">Models</h2><div class="desktop">` + table + `</div><div class="mobile">` + table + `</div>`,
		`<div class="desktop"><h2 id="models">Models</h2>` + table + `</div><div class="mobile"><h2 id="models">Models</h2>` + table + `</div>`,
	} {
		got, err := ParseDocument(strings.NewReader(doc))
		if err != nil || len(got) != 1 || got[0].ID != "cline-pass/a" {
			t.Fatalf("models = %#v, error = %v", got, err)
		}
	}
}

func TestParseDocumentRejectsInvalidOrIncompleteDocuments(t *testing.T) {
	valid := documentTestRow("Model A", "cline-pass/a")
	cases := map[string]string{
		"empty":               "",
		"error page":          `<html><h1>Service Unavailable</h1><p>Try again later</p></html>`,
		"table without title": documentTestTable(valid),
		"only title":          `<h2 id="models">Models</h2>`,
		"empty table":         `<h2 id="models">Models</h2>` + documentTestTable(""),
		"wrong headers":       `<h2 id="models">Models</h2><table><tr><th>Model</th><th>Price</th></tr>` + valid + `</table>`,
		"title in nav":        `<nav><h2>Models</h2>` + documentTestTable(valid) + `</nav>`,
		"table in template":   `<h2>Models</h2><template>` + documentTestTable(valid) + `</template>`,
		"following section":   `<h2>Models</h2><h2>Deprecated models</h2>` + documentTestTable(valid),
		"truncated table":     `<h2>Models</h2>` + documentTableStart + valid,
		"truncated last row":  `<h2>Models</h2>` + documentTableStart + valid + `<tr><td>Model B</td><td><code>cline-pass/b`,
		"duplicate conflict":  `<h2>Models</h2>` + documentTestTable(valid+documentTestRow("Other name", "cline-pass/a")),
		"conflicting tables":  `<h2>Models</h2>` + documentTestTable(valid) + documentTestTable(documentTestRow("Model B", "cline-pass/b")),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseDocument(strings.NewReader(doc))
			if err == nil || got != nil {
				t.Fatalf("models = %#v, error = %v; want nil models and error", got, err)
			}
		})
	}
}

func TestParseDocumentBadRowRejectsWholeUpdate(t *testing.T) {
	valid := documentTestRow("Model A", "cline-pass/a")
	cases := map[string]string{
		"missing cell":      `<tr><td>Model B</td></tr>`,
		"extra cell":        `<tr><td>Model B</td><td><code>cline-pass/b</code></td><td>Extra</td></tr>`,
		"missing name":      documentTestRow("  ", "cline-pass/b"),
		"missing ID":        documentTestRow("Model B", ""),
		"wrong provider":    documentTestRow("Model B", "another/b"),
		"nested path":       documentTestRow("Model B", "cline-pass/b/nested"),
		"space in ID":       documentTestRow("Model B", "cline-pass/model b"),
		"missing code":      `<tr><td>Model B</td><td>cline-pass/b</td></tr>`,
		"two codes":         `<tr><td>Model B</td><td><code>cline-pass/b</code><code>cline-pass/c</code></td></tr>`,
		"trailing ID text":  `<tr><td>Model B</td><td><code>cline-pass/b</code> or something else</td></tr>`,
		"merged cells":      `<tr><td rowspan="2">Model B</td><td><code>cline-pass/b</code></td></tr>`,
		"incomplete repair": `<tr><td>Model B</td>`,
	}
	for name, badRow := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseDocument(strings.NewReader(`<h2>Models</h2>` + documentTestTable(valid+badRow)))
			if err == nil || got != nil {
				t.Fatalf("models = %#v, error = %v; want whole update rejected", got, err)
			}
		})
	}
}

func TestParseDocumentReadFailures(t *testing.T) {
	if _, err := ParseDocument(io.MultiReader(strings.NewReader(`<h2>Models</h2>`), documentFailReader{})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
	if _, err := ParseDocument(strings.NewReader(strings.Repeat(" ", maxParsedDocumentBytes+1))); err == nil {
		t.Fatal("oversized document accepted")
	}
}

type documentFailReader struct{}

func (documentFailReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
