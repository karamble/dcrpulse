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
