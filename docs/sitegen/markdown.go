package main

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"
)

// rendered is one markdown file turned into HTML. Title is the first heading.
type rendered struct {
	Title       string
	Description string
	HTML        string
}

// renderMarkdown turns a small markdown subset into HTML. rewrite maps a link
// destination to the URL that should be written. Raw HTML in the source is
// escaped, so a docs page cannot inject markup.
func renderMarkdown(src string, rewrite func(string) string) rendered {
	if rewrite == nil {
		rewrite = func(dest string) string { return dest }
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out strings.Builder
	var title, description string
	used := map[string]int{}
	i := 0
	for i < len(lines) {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if trim == "" || isComment(trim) {
			i++
			continue
		}
		if strings.HasPrefix(trim, "```") {
			var buf []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				buf = append(buf, lines[i])
				i++
			}
			if i < len(lines) {
				i++
			}
			out.WriteString("<pre><code>")
			out.WriteString(escapeText(strings.Join(buf, "\n")))
			if len(buf) > 0 {
				out.WriteByte('\n')
			}
			out.WriteString("</code></pre>\n")
			continue
		}
		if isRule(trim) {
			out.WriteString("<hr>\n")
			i++
			continue
		}
		if level, text, ok := heading(trim); ok {
			plain := plainText(text)
			if title == "" && level == 1 {
				title = plain
			}
			id := uniqueSlug(used, plain)
			out.WriteString("<h" + strconv.Itoa(level) + " id=\"" + id + "\">")
			out.WriteString(inline(text, rewrite))
			out.WriteString("</h" + strconv.Itoa(level) + ">\n")
			i++
			continue
		}
		if strings.HasPrefix(trim, "|") && i+1 < len(lines) && isSeparator(strings.TrimSpace(lines[i+1])) {
			header := splitRow(trim)
			i += 2
			var rows [][]string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				rows = append(rows, splitRow(strings.TrimSpace(lines[i])))
				i++
			}
			out.WriteString("<div class=\"table-wrap\"><table>\n<thead><tr>")
			for _, cell := range header {
				out.WriteString("<th scope=\"col\">")
				out.WriteString(inline(strings.TrimSpace(cell), rewrite))
				out.WriteString("</th>")
			}
			out.WriteString("</tr></thead>\n<tbody>\n")
			for _, row := range rows {
				out.WriteString("<tr>")
				for _, cell := range row {
					out.WriteString("<td>")
					out.WriteString(inline(strings.TrimSpace(cell), rewrite))
					out.WriteString("</td>")
				}
				out.WriteString("</tr>\n")
			}
			out.WriteString("</tbody></table></div>\n")
			continue
		}
		if kind, item, ok := listItem(trim); ok {
			tag := "ul"
			if kind == ordered {
				tag = "ol"
			}
			out.WriteString("<" + tag + ">\n")
			for {
				out.WriteString("<li>")
				out.WriteString(inline(item, rewrite))
				out.WriteString("</li>\n")
				i++
				if i >= len(lines) {
					break
				}
				nextKind, nextItem, nextOK := listItem(strings.TrimSpace(lines[i]))
				if !nextOK || nextKind != kind {
					break
				}
				item = nextItem
			}
			out.WriteString("</" + tag + ">\n")
			continue
		}
		if strings.HasPrefix(trim, "> ") || trim == ">" {
			var parts []string
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if cur == ">" {
					parts = append(parts, "")
					i++
					continue
				}
				if !strings.HasPrefix(cur, "> ") {
					break
				}
				parts = append(parts, strings.TrimPrefix(cur, "> "))
				i++
			}
			text := strings.Join(parts, " ")
			if description == "" {
				description = clip(plainText(text), 160)
			}
			out.WriteString("<blockquote><p>")
			out.WriteString(inline(text, rewrite))
			out.WriteString("</p></blockquote>\n")
			continue
		}
		var parts []string
		for i < len(lines) {
			cur := strings.TrimSpace(lines[i])
			if cur == "" || startsBlock(cur) {
				break
			}
			parts = append(parts, cur)
			i++
		}
		text := strings.Join(parts, " ")
		if description == "" {
			description = clip(plainText(text), 160)
		}
		out.WriteString("<p>")
		out.WriteString(inline(text, rewrite))
		out.WriteString("</p>\n")
	}
	return rendered{Title: title, Description: description, HTML: out.String()}
}

func startsBlock(trim string) bool {
	if strings.HasPrefix(trim, "```") || isRule(trim) || strings.HasPrefix(trim, "|") || strings.HasPrefix(trim, "> ") || trim == ">" {
		return true
	}
	if _, _, ok := heading(trim); ok {
		return true
	}
	_, _, ok := listItem(trim)
	return ok
}

func isComment(trim string) bool {
	return strings.HasPrefix(trim, "<!--") && strings.HasSuffix(trim, "-->")
}

func isRule(trim string) bool {
	return trim == "---" || trim == "***" || trim == "___"
}

func heading(trim string) (int, string, bool) {
	if !strings.HasPrefix(trim, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trim) && trim[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(trim) || trim[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trim[level+1:]), true
}

type listKind int

const (
	bullet listKind = iota
	ordered
)

func listItem(trim string) (listKind, string, bool) {
	if strings.HasPrefix(trim, "- ") {
		return bullet, strings.TrimSpace(trim[2:]), true
	}
	n := 0
	for n < len(trim) && trim[n] >= '0' && trim[n] <= '9' {
		n++
	}
	if n > 0 && n+2 <= len(trim) && trim[n] == '.' && trim[n+1] == ' ' {
		return ordered, strings.TrimSpace(trim[n+2:]), true
	}
	return 0, "", false
}

func isSeparator(trim string) bool {
	cells := splitRow(trim)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		cell = strings.Trim(cell, ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func splitRow(trim string) []string {
	trim = strings.TrimSpace(trim)
	trim = strings.TrimPrefix(trim, "|")
	trim = strings.TrimSuffix(trim, "|")
	return strings.Split(trim, "|")
}

func inline(s string, rewrite func(string) string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				b.WriteString("<code>")
				b.WriteString(escapeText(s[i+1 : i+1+end]))
				b.WriteString("</code>")
				i += end + 2
				continue
			}
		}
		if s[i] == '!' && i+1 < len(s) && s[i+1] == '[' {
			if text, dest, next, ok := parseLink(s[i+1:]); ok {
				b.WriteString("<img src=\"")
				b.WriteString(html.EscapeString(rewrite(dest)))
				b.WriteString("\" alt=\"")
				b.WriteString(html.EscapeString(text))
				b.WriteString("\">")
				i += 1 + next
				continue
			}
		}
		if s[i] == '[' {
			if text, dest, next, ok := parseLink(s[i:]); ok {
				b.WriteString("<a href=\"")
				b.WriteString(html.EscapeString(rewrite(dest)))
				b.WriteString("\">")
				b.WriteString(inline(text, rewrite))
				b.WriteString("</a>")
				i += next
				continue
			}
		}
		if strings.HasPrefix(s[i:], "**") {
			if end := strings.Index(s[i+2:], "**"); end >= 0 {
				b.WriteString("<strong>")
				b.WriteString(inline(s[i+2:i+2+end], rewrite))
				b.WriteString("</strong>")
				i += end + 4
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(escapeText(string(r)))
		i += size
	}
	return b.String()
}

// escapeText escapes the characters that start markup. Quotes stay as quotes
// so a sentence like "isn't coming in" is the sentence the product shows.
func escapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseLink reads [text](dest) at the start of s. next is the number of bytes consumed.
func parseLink(s string) (text, dest string, next int, ok bool) {
	if !strings.HasPrefix(s, "[") {
		return "", "", 0, false
	}
	closeText := strings.IndexByte(s, ']')
	if closeText < 0 || closeText+1 >= len(s) || s[closeText+1] != '(' {
		return "", "", 0, false
	}
	closeDest := strings.IndexByte(s[closeText+2:], ')')
	if closeDest < 0 {
		return "", "", 0, false
	}
	text = s[1:closeText]
	dest = s[closeText+2 : closeText+2+closeDest]
	return text, dest, closeText + 2 + closeDest + 1, true
}

func plainText(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], " ")
	if cut < 40 {
		cut = n
	}
	return s[:cut] + "…"
}

func uniqueSlug(used map[string]int, text string) string {
	base := slug(text)
	if base == "" {
		base = "section"
	}
	n := used[base]
	used[base] = n + 1
	if n == 0 {
		return base
	}
	return base + "-" + strconv.Itoa(n+1)
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
