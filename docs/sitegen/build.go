package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed docs.css
var assetCSS embed.FS

// handbook is the public guide, in nav order. Each file lives in docs/guide.
var handbook = []struct {
	File  string
	Label string
}{
	{"index.md", "Home"},
	{"install.md", "Install"},
	{"first-run.md", "First run"},
	{"tuners.md", "Tuners"},
	{"channels.md", "Channels and the guide"},
	{"watching.md", "Watching"},
	{"multiview.md", "Multiview"},
	{"recordings.md", "Recordings and storage"},
	{"sync.md", "Sync"},
	{"troubleshooting.md", "Troubleshooting"},
	{"faq.md", "FAQ"},
	{"security.md", "Security"},
}

// notes are longer repository pages. A missing file is left out, except that
// docs/security.md is linked from the Security page when it is present.
var notes = []struct {
	File  string
	Label string
}{
	{"troubleshooting.md", "Messages"},
	{"tuners.md", "Finding tuners"},
	{"recordings.md", "How recordings are stored"},
	{"multiview.md", "Multiview layouts"},
	{"atsc3.md", "ATSC 3.0"},
	{"sources.md", "Sources"},
	{"security.md", "Security notes"},
}

const securitySentence = "These checks are also written up in"

type docPage struct {
	source string
	out    string
	label  string
	group  string
}

// Build writes a static site for docsRoot into outDir. outDir is replaced.
// It must not be the docs directory or a git checkout.
func Build(docsRoot, outDir string) error {
	docsRoot, outDir, err := cleanRoots(docsRoot, outDir)
	if err != nil {
		return err
	}
	pages, err := collect(docsRoot)
	if err != nil {
		return err
	}
	bySource := map[string]docPage{}
	for _, p := range pages {
		bySource[p.source] = p
	}
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0o755); err != nil {
		return err
	}
	css, err := assetCSS.ReadFile("docs.css")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "assets", "docs.css"), css, 0o644); err != nil {
		return err
	}
	for _, p := range pages {
		body, err := os.ReadFile(filepath.Join(docsRoot, filepath.FromSlash(p.source)))
		if err != nil {
			return err
		}
		rewrite := func(dest string) string {
			return rewriteLink(docsRoot, bySource, p.source, dest)
		}
		doc := renderMarkdown(string(body), rewrite)
		if p.source == "guide/security.md" {
			doc.HTML = linkSecurityNote(doc.HTML, bySource)
		}
		title := doc.Title
		if title == "" {
			title = p.label
		}
		description := doc.Description
		if description == "" {
			description = "Live TV and DVR for an antenna."
		}
		var buf bytes.Buffer
		data := pageData{
			DocTitle:    docTitle(title),
			Description: description,
			CSS:         hrefBetween(p.out, "assets/docs.css"),
			Home:        hrefBetween(p.out, "index.html"),
			Groups:      navGroups(pages, p.out),
			Body:        template.HTML(doc.HTML),
		}
		if err := pageTmpl.Execute(&buf, data); err != nil {
			return err
		}
		dest := filepath.Join(outDir, filepath.FromSlash(p.out))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func collect(docsRoot string) ([]docPage, error) {
	var pages []docPage
	for _, item := range handbook {
		source := path.Join("guide", item.File)
		if _, err := os.Stat(filepath.Join(docsRoot, filepath.FromSlash(source))); err != nil {
			return nil, fmt.Errorf("missing docs/%s", source)
		}
		out := "index.html"
		if item.File != "index.md" {
			out = strings.TrimSuffix(item.File, ".md") + ".html"
		}
		pages = append(pages, docPage{source: source, out: out, label: item.Label, group: "Guide"})
	}
	for _, item := range notes {
		if _, err := os.Stat(filepath.Join(docsRoot, item.File)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		pages = append(pages, docPage{
			source: item.File,
			out:    path.Join("reference", strings.TrimSuffix(item.File, ".md")+".html"),
			label:  item.Label,
			group:  "Notes",
		})
	}
	return pages, nil
}

// linkSecurityNote points the Security guide page at docs/security.md when
// that file was rendered. The sentence is omitted when the note is absent,
// so the page does not link to a file the repository does not have.
func linkSecurityNote(body string, bySource map[string]docPage) string {
	note, ok := bySource["security.md"]
	if !ok {
		return body
	}
	href := hrefBetween("security.html", note.out)
	needle := `href="` + href + `"`
	if strings.Contains(body, needle) {
		return body
	}
	return body + "<p>" + securitySentence + " <a href=\"" + html.EscapeString(href) + "\">Security notes</a>.</p>\n"
}

func rewriteLink(docsRoot string, bySource map[string]docPage, from, dest string) string {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
		return dest
	}
	pathPart, frag, _ := strings.Cut(dest, "#")
	if pathPart == "" {
		if frag != "" {
			return "#" + frag
		}
		return dest
	}
	if strings.HasPrefix(pathPart, "/") {
		return dest
	}
	joined := path.Clean(path.Join(path.Dir(from), pathPart))
	if to, ok := bySource[joined]; ok {
		href := hrefBetween(bySource[from].out, to.out)
		if frag != "" {
			href += "#" + frag
		}
		return href
	}
	if strings.HasSuffix(strings.ToLower(joined), ".md") {
		disk := filepath.Join(docsRoot, filepath.FromSlash(joined))
		if st, err := os.Stat(disk); err == nil && !st.IsDir() {
			href := "https://github.com/wolfebase/broadwave/blob/main/docs/" + joined
			if frag != "" {
				href += "#" + frag
			}
			return href
		}
	}
	return dest
}

func hrefBetween(fromFile, toFile string) string {
	fromDir := filepath.Dir(filepath.FromSlash(fromFile))
	rel, err := filepath.Rel(fromDir, filepath.FromSlash(toFile))
	if err != nil {
		return toFile
	}
	return filepath.ToSlash(rel)
}

func docTitle(title string) string {
	if title == "Broadwave" {
		return title
	}
	return title + " · Broadwave"
}

func cleanRoots(docsRoot, outDir string) (string, string, error) {
	var err error
	docsRoot, err = filepath.Abs(docsRoot)
	if err != nil {
		return "", "", err
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return "", "", err
	}
	if outDir == docsRoot {
		return "", "", errors.New("refusing to replace the docs directory")
	}
	if _, err := os.Stat(filepath.Join(outDir, ".git")); err == nil {
		return "", "", errors.New("refusing to write over a git checkout")
	}
	rel, err := filepath.Rel(outDir, docsRoot)
	if err != nil {
		return "", "", err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", errors.New("refusing to write over a parent of the docs directory")
	}
	// A direct child of docs is the guide or this tool. The site goes deeper, or somewhere else.
	inside, err := filepath.Rel(docsRoot, outDir)
	if err != nil {
		return "", "", err
	}
	if !strings.HasPrefix(inside, "..") && !strings.Contains(inside, string(os.PathSeparator)) {
		return "", "", errors.New("refusing to replace a directory directly inside docs")
	}
	return docsRoot, outDir, nil
}

type pageData struct {
	DocTitle    string
	Description string
	CSS         string
	Home        string
	Groups      []navGroup
	Body        template.HTML
}

type navGroup struct {
	Label string
	Items []navItem
}

type navItem struct {
	Href    string
	Label   string
	Current bool
}

func navGroups(pages []docPage, current string) []navGroup {
	order := []string{"Guide", "Notes"}
	grouped := map[string][]navItem{}
	for _, p := range pages {
		grouped[p.group] = append(grouped[p.group], navItem{
			Href:    hrefBetween(current, p.out),
			Label:   p.label,
			Current: p.out == current,
		})
	}
	var groups []navGroup
	for _, name := range order {
		items := grouped[name]
		if len(items) == 0 {
			continue
		}
		groups = append(groups, navGroup{Label: name, Items: items})
	}
	return groups
}

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.DocTitle}}</title>
<meta name="description" content="{{.Description}}">
<link rel="stylesheet" href="{{.CSS}}">
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='8' fill='%233D7BFF'/%3E%3C/svg%3E">
</head>
<body>
<a class="skip" href="#content">Skip to content</a>
<header>
<div class="wrap">
<a class="brand" href="{{.Home}}">Broadwave</a>
<p class="header-note">Docs</p>
</div>
</header>
<div class="layout">
<nav aria-label="Documentation">
{{range .Groups}}<p class="nav-label">{{.Label}}</p>
<ul>
{{range .Items}}<li><a href="{{.Href}}"{{if .Current}} aria-current="page"{{end}}>{{.Label}}</a></li>
{{end}}</ul>
{{end}}</nav>
<main id="content">
{{.Body}}<footer><p><a href="https://github.com/wolfebase/broadwave">Source</a> · <a href="https://github.com/wolfebase/broadwave/blob/main/LICENSE">Apache 2.0</a></p></footer>
</main>
</div>
</body>
</html>
`))
