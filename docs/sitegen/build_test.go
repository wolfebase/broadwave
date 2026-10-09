package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSiteBuildsTheGuide(t *testing.T) {
	root := docsDir(t)
	out := t.TempDir()
	if err := Build(root, out); err != nil {
		t.Fatal(err)
	}

	index := readOut(t, out, "index.html")
	for _, item := range handbook {
		name := "index.html"
		if item.File != "index.md" {
			name = strings.TrimSuffix(item.File, ".md") + ".html"
		}
		page := readOut(t, out, name)
		if strings.Count(page, "<h1") != 1 {
			t.Errorf("%s: want one h1", name)
		}
		for _, needle := range []string{
			`<html lang="en">`,
			`name="viewport"`,
			`<a class="skip" href="#content">`,
			`<main id="content">`,
			`<nav aria-label="Documentation">`,
			`aria-current="page"`,
		} {
			if !strings.Contains(page, needle) {
				t.Errorf("%s: missing %s", name, needle)
			}
		}
		if !strings.Contains(index, "href=\""+name+"\"") && name != "index.html" {
			t.Errorf("home nav missing %s", name)
		}
		if strings.Contains(page, "192.168.") || strings.Contains(page, "@gmail.com") {
			t.Errorf("%s: contains a private address", name)
		}
		css := "assets/docs.css"
		if strings.HasPrefix(name, "reference/") {
			css = "../assets/docs.css"
		}
		if !strings.Contains(page, `href="`+css+`"`) {
			t.Errorf("%s: stylesheet link is wrong", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "docs.css")); err != nil {
		t.Fatal(err)
	}
	css := readOut(t, out, "assets/docs.css")
	if !strings.Contains(css, ".skip:not(:focus)") || !strings.Contains(css, "prefers-reduced-motion") {
		t.Fatal("stylesheet is missing the skip link or reduced motion")
	}
	if !strings.Contains(readOut(t, out, "index.html"), `<p class="nav-label">Guide</p>`) {
		t.Fatal("nav is missing the guide label")
	}
	if !strings.Contains(css, "max-height: 40vh") {
		t.Fatal("stylesheet does not keep the narrow nav on screen")
	}

	want := map[string][]string{
		"install.html":          {"Unraid", "Docker", "8477", "ghcr.io/wolfebase/broadwave", "PUID"},
		"first-run.html":        {"Let's set up your TV", "Add by address"},
		"tuners.html":           {"HDHomeRun", "FLEX 4K", "ATSC 3.0", "reference/atsc3.html"},
		"channels.html":         {"Schedules Direct", "XMLTV"},
		"watching.html":         {"iPhone", "iPad", "Apple TV", "8477", "web", "saved on that device"},
		"multiview.html":        {"Side by side", "Quad"},
		"recordings.html":       {"MPEG-TS", "/config/work/recordings", "Keep this much free", "last 15 seconds", "longer than 90 seconds", "first sound track"},
		"sync.html":             {"Whole-Home Sync", "Watch together"},
		"troubleshooting.html":  {"isn't coming in", "Diagnostics", "No tuner answered yet"},
		"faq.html":              {"antenna", "FLEX 4K"},
		"security.html":         {"BROADWAVE_HOSTS", securitySentence, "reference/security.html", "port 443"},
		"reference/tuners.html": {"Search the network"},
		"reference/atsc3.html":  {"FLEX 4K"},
	}
	for name, needles := range want {
		page := readOut(t, out, name)
		for _, needle := range needles {
			if !strings.Contains(page, needle) {
				t.Errorf("%s: missing %q", name, needle)
			}
		}
	}
	if strings.Contains(readOut(t, out, "install.html"), "<your-server>") {
		t.Fatal("install page left a placeholder unescaped")
	}
	sources := readOut(t, out, "reference/sources.html")
	if strings.Contains(sources, "<freq>") || !strings.Contains(sources, "&lt;freq&gt;") {
		t.Fatal("source table did not escape <freq>")
	}
	if !strings.Contains(sources, "/playlist/channels") || !strings.Contains(sources, "Not a source") {
		t.Fatal("sources page lost the tvheadend path or the Tablo status")
	}
	if !strings.Contains(sources, "https://github.com/wolfebase/broadwave/blob/main/docs/decisions/0009-sources-and-discovery.md") {
		t.Fatal("decision link should point at the repository")
	}
	mv := readOut(t, out, "reference/multiview.html")
	if !strings.Contains(mv, "https://github.com/wolfebase/broadwave/blob/main/docs/hardware.md") || strings.Contains(mv, "hardware.html") {
		t.Fatal("hardware.md should stay on GitHub")
	}
	for _, name := range []string{"reference/research.html", "reference/hardware.html"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err == nil {
			t.Fatalf("published %s", name)
		}
	}
	blob := readOut(t, out, "reference/troubleshooting.html") + readOut(t, out, "reference/recordings.html")
	if strings.Contains(blob, "This house has") || strings.Contains(blob, "the user's lineup") {
		t.Fatal("published a private note")
	}
}

func TestSecurityLinkFollowsTheNote(t *testing.T) {
	root := t.TempDir()
	guide := filepath.Join(root, "guide")
	if err := os.MkdirAll(guide, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, item := range handbook {
		body := "# " + item.Label + "\n\nA page.\n"
		if err := os.WriteFile(filepath.Join(guide, item.File), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	if err := Build(root, out); err != nil {
		t.Fatal(err)
	}
	without := readOut(t, out, "security.html")
	if strings.Contains(without, "reference/security.html") || strings.Contains(without, securitySentence) {
		t.Fatal("linked a security note that is not in the tree")
	}

	if err := os.WriteFile(filepath.Join(root, "security.md"), []byte("# Security\n\nDeviceAuth is not stored.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Build(root, out); err != nil {
		t.Fatal(err)
	}
	with := readOut(t, out, "security.html")
	if !strings.Contains(with, securitySentence) || !strings.Contains(with, `href="reference/security.html"`) {
		t.Fatal("security page did not link the repository note")
	}
	if !strings.Contains(readOut(t, out, "reference/security.html"), "DeviceAuth is not stored.") {
		t.Fatal("security note was not rendered")
	}
}

func TestLocalLinksResolve(t *testing.T) {
	root := docsDir(t)
	out := t.TempDir()
	if err := Build(root, out); err != nil {
		t.Fatal(err)
	}
	hrefs := regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	ids := regexp.MustCompile(`\sid="([^"]+)"`)
	var files []string
	err := filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".html") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	pageIDs := map[string]map[string]bool{}
	bodies := map[string]string{}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		bodies[file] = string(body)
		found := map[string]bool{}
		for _, m := range ids.FindAllStringSubmatch(string(body), -1) {
			found[m[1]] = true
		}
		pageIDs[file] = found
	}
	for _, file := range files {
		for _, m := range hrefs.FindAllStringSubmatch(bodies[file], -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "http:") || strings.HasPrefix(ref, "https:") || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "mailto:") {
				continue
			}
			pathPart, frag, _ := strings.Cut(ref, "#")
			target := file
			if pathPart != "" {
				target = filepath.Clean(filepath.Join(filepath.Dir(file), filepath.FromSlash(pathPart)))
				if _, err := os.Stat(target); err != nil {
					t.Errorf("%s links to missing %s", filepath.Base(file), ref)
					continue
				}
			}
			if frag != "" {
				if !pageIDs[target][frag] {
					t.Errorf("%s links to missing #%s on %s", filepath.Base(file), frag, filepath.Base(target))
				}
			}
		}
	}
}

func TestMissingGuidePageFails(t *testing.T) {
	root := t.TempDir()
	guide := filepath.Join(root, "guide")
	if err := os.MkdirAll(guide, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, item := range handbook {
		if item.File == "install.md" {
			continue
		}
		if err := os.WriteFile(filepath.Join(guide, item.File), []byte("# "+item.Label+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := Build(root, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "guide/install.md") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildRefusesToReplaceDocs(t *testing.T) {
	root := docsDir(t)
	if err := Build(root, root); err == nil {
		t.Fatal("replaced the docs directory")
	}
	if err := Build(root, filepath.Join(root, "guide")); err == nil {
		t.Fatal("replaced docs/guide")
	}
}

func TestMarkdownTableCodeAndLink(t *testing.T) {
	doc := renderMarkdown(strings.Join([]string{
		"# Title",
		"",
		"See [the note](../security.md) and `a<b`.",
		"",
		"| Tuner | What |",
		"| --- | --- |",
		"| FLEX 4K | Two of four |",
		"",
		"```",
		"docker run <image>",
		"```",
		"",
		"- One",
		"- Two",
	}, "\n"), func(dest string) string {
		if dest == "../security.md" {
			return "reference/security.html"
		}
		return dest
	})
	if doc.Title != "Title" {
		t.Fatalf("title %q", doc.Title)
	}
	for _, needle := range []string{
		`<a href="reference/security.html">the note</a>`,
		"<code>a&lt;b</code>",
		"<th scope=\"col\">Tuner</th>",
		"<td>Two of four</td>",
		"<pre><code>docker run &lt;image&gt;\n</code></pre>",
		"<li>One</li>",
	} {
		if !strings.Contains(doc.HTML, needle) {
			t.Errorf("missing %s\n%s", needle, doc.HTML)
		}
	}
}

func TestDescriptionDropsMarkdown(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(docsDir(t), "sources.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := renderMarkdown(string(body), nil)
	if strings.Contains(doc.Description, "](") || strings.Contains(doc.Description, "[") {
		t.Fatalf("description kept markdown: %q", doc.Description)
	}
	if !strings.Contains(doc.Description, "0009") {
		t.Fatalf("description dropped the link text: %q", doc.Description)
	}
}

func TestPipeLineWithoutTableDoesNotHang(t *testing.T) {
	done := make(chan rendered, 1)
	go func() {
		done <- renderMarkdown(strings.Join([]string{
			"# Title",
			"",
			"| not a table",
			"",
			"Still here.",
			"",
			"| last line",
		}, "\n"), nil)
	}()
	select {
	case doc := <-done:
		if !strings.Contains(doc.HTML, "Still here") {
			t.Fatalf("did not advance past the pipe line:\n%s", doc.HTML)
		}
		if !strings.Contains(doc.HTML, "not a table") || !strings.Contains(doc.HTML, "last line") {
			t.Fatalf("dropped a pipe line:\n%s", doc.HTML)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("renderMarkdown hung on a pipe line that is not a table")
	}
}

func docsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func readOut(t *testing.T, out, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}
