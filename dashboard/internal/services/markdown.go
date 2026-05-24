// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Built once at first use; safe for concurrent use.
var (
	mdRendererOnce sync.Once
	mdMD           goldmark.Markdown
	mdPolicy       *bluemonday.Policy
	mdPagePolicy   *bluemonday.Policy
)

func initMDRenderer() {
	mdMD = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			html.WithXHTML(),
			html.WithHardWraps(),
		),
	)

	// Deny-by-default policy + explicit allow-list of the tags goldmark
	// emits for the markdown subset our consumers use today. <img> is
	// intentionally NOT allowed to keep the dashboard from silently
	// fetching third-party assets (matches Decrediton's
	// renderProposalImage behaviour for Politeia, and is conservative
	// enough for BR posts in the absence of a better policy).
	mdPolicy = bluemonday.StrictPolicy()
	mdPolicy.AllowStandardURLs()
	mdPolicy.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr",
		"strong", "em", "del",
		"code", "pre",
		"blockquote",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "th", "td",
		"a",
	)
	mdPolicy.AllowAttrs("href").OnElements("a")
	mdPolicy.AllowAttrs("class").OnElements("code", "pre")
	mdPolicy.RequireNoFollowOnLinks(true)
	mdPolicy.AddTargetBlankToFullyQualifiedLinks(true)

	// Pages reuse the same element allow-list as posts, but must keep br://
	// and relative links intact: the Pages viewer intercepts clicks on those
	// to navigate in-app (br://UID/path or a relative page path), while
	// http/https links open externally as usual.
	mdPagePolicy = bluemonday.StrictPolicy()
	mdPagePolicy.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr",
		"strong", "em", "del",
		"code", "pre",
		"blockquote",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "th", "td",
		"a",
	)
	mdPagePolicy.AllowAttrs("href").OnElements("a")
	mdPagePolicy.AllowAttrs("class").OnElements("code", "pre")
	mdPagePolicy.AllowURLSchemes("http", "https", "mailto", "br")
	mdPagePolicy.AllowRelativeURLs(true)
	mdPagePolicy.RequireNoFollowOnLinks(true)
	mdPagePolicy.AddTargetBlankToFullyQualifiedLinks(true)
}

// renderPageMarkdownHTML is RenderMarkdownHTML with the page link policy
// (br:// + relative hrefs preserved).
func renderPageMarkdownHTML(src string) string {
	if src == "" {
		return ""
	}
	mdRendererOnce.Do(initMDRenderer)
	var buf bytes.Buffer
	if err := mdMD.Convert([]byte(src), &buf); err != nil {
		log.Printf("page markdown render: %v", err)
		return ""
	}
	return string(mdPagePolicy.SanitizeBytes(buf.Bytes()))
}

// RenderMarkdownHTML converts CommonMark + GFM source to a sanitized
// HTML string. Returns "" if rendering fails; the caller may fall back
// to the raw source in that case. Shared between the Politeia proposal
// renderer and the Bison Relay post-body renderer (both want the same
// strict policy until proven otherwise).
func RenderMarkdownHTML(src string) string {
	if src == "" {
		return ""
	}
	mdRendererOnce.Do(initMDRenderer)
	var buf bytes.Buffer
	if err := mdMD.Convert([]byte(src), &buf); err != nil {
		log.Printf("markdown render: %v", err)
		return ""
	}
	return string(mdPolicy.SanitizeBytes(buf.Bytes()))
}

// BRPostBodySegment is one chunk of a BR post body — either a slab of
// rendered HTML (from a markdown text run) or an inline embed (image
// etc.) whose raw bytes are base64 in DataB64. Mirrors the segment
// shape the dashboard's chat MessageBody already understands client-side.
type BRPostBodySegment struct {
	Kind    string `json:"kind"` // "text" | "embed"
	HTML    string `json:"html,omitempty"`
	Name    string `json:"name,omitempty"`
	Mime    string `json:"mime,omitempty"`
	DataB64 string `json:"data_b64,omitempty"`
	Size    int    `json:"size,omitempty"`
	Alt     string `json:"alt,omitempty"`
}

// brPostEmbedRE matches BR's --embed[k=v,k=v]-- (and the parallel
// --download[...]-- chat-side tag, which we silently drop in posts).
var brPostEmbedRE = regexp.MustCompile(`--(embed|download)\[(.*?)\]--`)

// SplitAndRenderBRPostBody scans a BR post body for --embed[...]-- tags,
// rendering the markdown text between tags through RenderMarkdownHTML
// and emitting each embed as a structured segment so the frontend can
// render images / attachments inline. Download tags (chat-only) are
// skipped since BR doesn't deliver file content via the posts path.
func SplitAndRenderBRPostBody(src string) []BRPostBodySegment {
	if src == "" {
		return nil
	}
	matches := brPostEmbedRE.FindAllStringSubmatchIndex(src, -1)
	if len(matches) == 0 {
		html := RenderMarkdownHTML(src)
		if html == "" {
			return nil
		}
		return []BRPostBodySegment{{Kind: "text", HTML: html}}
	}
	out := make([]BRPostBodySegment, 0, len(matches)*2+1)
	last := 0
	for _, m := range matches {
		if m[0] > last {
			text := src[last:m[0]]
			if html := RenderMarkdownHTML(text); html != "" {
				out = append(out, BRPostBodySegment{Kind: "text", HTML: html})
			}
		}
		kind := src[m[2]:m[3]]
		inner := src[m[4]:m[5]]
		if kind == "embed" {
			out = append(out, parseBREmbedTag(inner))
		}
		// Drop --download[...]-- tags — those are chat-side file transfer
		// chips, not applicable in posts.
		last = m[1]
	}
	if last < len(src) {
		if html := RenderMarkdownHTML(src[last:]); html != "" {
			out = append(out, BRPostBodySegment{Kind: "text", HTML: html})
		}
	}
	return out
}

// BRPageSegment is one chunk of a rendered BR page. Like BRPostBodySegment
// it can be rendered text or an inline embed, but pages add two more kinds:
// "form" (an interactive --form-- block) and section tagging so the viewer
// can patch a single --section id=X-- region when an async form reply lands.
type BRPageSegment struct {
	Kind      string            `json:"kind"` // "text" | "embed" | "form"
	SectionID string            `json:"section_id,omitempty"`
	HTML      string            `json:"html,omitempty"`
	Name      string            `json:"name,omitempty"`
	Mime      string            `json:"mime,omitempty"`
	DataB64   string            `json:"data_b64,omitempty"`
	Size      int               `json:"size,omitempty"`
	Alt       string            `json:"alt,omitempty"`
	Fields    []BRPageFormField `json:"fields,omitempty"`
}

// BRPageFormField mirrors bruig's FormField (components/pages/forms.dart):
// fields are parsed from key="value" pairs, with type selecting the control.
// Known types: txtinput, intinput, submit, action, asynctarget, hidden.
type BRPageFormField struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Label     string `json:"label,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Value     string `json:"value,omitempty"`
	Regexp    string `json:"regexp,omitempty"`
	RegexpStr string `json:"regexpstr,omitempty"`
}

var (
	// pageSectionStartRE matches bruig's --section id=ID -- start marker
	// (note the trailing " --"); pages emit these on their own line.
	pageSectionStartRE = regexp.MustCompile(`^--section id=(\w+) --$`)
	// pageFormFieldRE matches each key="value" pair on a form field line.
	pageFormFieldRE = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// SplitAndRenderBRPage turns BR page markdown into structured segments: text
// runs rendered to HTML (br:// + relative links preserved), inline --embed--
// images/attachments, and interactive --form-- blocks. Each segment is tagged
// with the enclosing --section id=X-- (empty when top-level) so the viewer can
// patch one section in place when an async form reply targets it.
func SplitAndRenderBRPage(src string) []BRPageSegment {
	if src == "" {
		return nil
	}
	var out []BRPageSegment
	section := ""
	var textBuf []string
	inForm := false
	var formLines []string

	flushText := func() {
		if len(textBuf) == 0 {
			return
		}
		out = append(out, renderPageTextRun(strings.Join(textBuf, "\n"), section)...)
		textBuf = textBuf[:0]
	}

	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case inForm:
			if line == "--/form--" {
				inForm = false
				out = append(out, parsePageForm(formLines, section))
				formLines = formLines[:0]
				continue
			}
			formLines = append(formLines, line)
		case line == "--form--":
			flushText()
			inForm = true
		case pageSectionStartRE.MatchString(line):
			flushText()
			section = pageSectionStartRE.FindStringSubmatch(line)[1]
		case line == "--/section--":
			flushText()
			section = ""
		default:
			textBuf = append(textBuf, raw)
		}
	}
	flushText()
	// Tolerate an unterminated form by still emitting what we parsed.
	if inForm && len(formLines) > 0 {
		out = append(out, parsePageForm(formLines, section))
	}
	return out
}

// renderPageTextRun splits a non-form text run by inline --embed-- tags,
// rendering the markdown between them and tagging every segment with section.
func renderPageTextRun(text, section string) []BRPageSegment {
	matches := brPostEmbedRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		html := renderPageMarkdownHTML(text)
		if html == "" {
			return nil
		}
		return []BRPageSegment{{Kind: "text", SectionID: section, HTML: html}}
	}
	out := make([]BRPageSegment, 0, len(matches)*2+1)
	last := 0
	for _, m := range matches {
		if m[0] > last {
			if html := renderPageMarkdownHTML(text[last:m[0]]); html != "" {
				out = append(out, BRPageSegment{Kind: "text", SectionID: section, HTML: html})
			}
		}
		if text[m[2]:m[3]] == "embed" {
			emb := parseBREmbedTag(text[m[4]:m[5]])
			out = append(out, BRPageSegment{
				Kind: "embed", SectionID: section,
				Name: emb.Name, Mime: emb.Mime, DataB64: emb.DataB64, Size: emb.Size, Alt: emb.Alt,
			})
		}
		last = m[1]
	}
	if last < len(text) {
		if html := renderPageMarkdownHTML(text[last:]); html != "" {
			out = append(out, BRPageSegment{Kind: "text", SectionID: section, HTML: html})
		}
	}
	return out
}

// parsePageForm builds a form segment from the field lines between --form--
// and --/form--. Mirrors FormBlockSyntax.parse: each line is a set of
// key="value" pairs, with type selecting the control.
func parsePageForm(lines []string, section string) BRPageSegment {
	seg := BRPageSegment{Kind: "form", SectionID: section}
	for _, line := range lines {
		pairs := pageFormFieldRE.FindAllStringSubmatch(line, -1)
		if len(pairs) == 0 {
			continue
		}
		var f BRPageFormField
		for _, p := range pairs {
			switch p[1] {
			case "type":
				f.Type = p[2]
			case "name":
				f.Name = p[2]
			case "label":
				f.Label = p[2]
			case "hint":
				f.Hint = p[2]
			case "value":
				f.Value = p[2]
			case "regexp":
				f.Regexp = p[2]
			case "regexpstr":
				f.RegexpStr = p[2]
			}
		}
		if f.Type == "" {
			continue
		}
		seg.Fields = append(seg.Fields, f)
	}
	return seg
}

func parseBREmbedTag(inner string) BRPostBodySegment {
	seg := BRPostBodySegment{Kind: "embed"}
	for _, part := range strings.Split(inner, ",") {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		k := part[:eq]
		v := part[eq+1:]
		switch k {
		case "name":
			seg.Name = v
		case "type":
			seg.Mime = v
		case "data":
			seg.DataB64 = v
		case "size":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				seg.Size = n
			}
		case "alt":
			if dec, err := url.QueryUnescape(v); err == nil {
				seg.Alt = dec
			} else {
				seg.Alt = v
			}
		}
	}
	return seg
}
