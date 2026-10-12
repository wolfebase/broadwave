package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// caTags are the public Community Apps template tags
// (https://ca.unraid.net/submit/help/xml-field-reference) plus the Unraid
// dockerMan fields a template still needs in order to install.
var caTags = map[string]bool{
	"Name": true, "Repository": true, "Registry": true, "Network": true,
	"MyIP": true, "Shell": true, "Privileged": true, "Support": true,
	"Project": true, "Overview": true, "Description": true, "Category": true,
	"WebUI": true, "TemplateURL": true, "Icon": true, "ExtraParams": true,
	"PostArgs": true, "CPUset": true, "DateInstalled": true, "DonateText": true,
	"DonateLink": true, "Requires": true, "Config": true, "ReadMe": true,
	"License": true, "MinVer": true, "MaxVer": true, "Beta": true,
}

// caRecommended is what the parser asks for on a submission, beyond Name and Repository.
var caRecommended = []string{"Category", "Icon", "Overview", "Project", "Support", "TemplateURL"}

var (
	categoryRE = regexp.MustCompile(`^[A-Za-z]+(?::[A-Za-z]+)?(?: [A-Za-z]+(?::[A-Za-z]+)?)*$`)
	httpsURL   = regexp.MustCompile(`^https://[^\s]+$`)
)

type templateEl struct {
	name  string
	attrs map[string]string
	text  string
}

func TestCommunityAppsTemplates(t *testing.T) {
	root := filepath.Dir(docsDir(t))
	icon := "web/public/icons/icon-512.png"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(icon))); err != nil {
		t.Fatalf("icon %s: %v", icon, err)
	}
	broadwave := parseTemplate(t, filepath.Join(root, "deploy/unraid/broadwave.xml"))
	updater := parseTemplate(t, filepath.Join(root, "deploy/unraid/broadwave-updater.xml"))
	for _, els := range [][]templateEl{broadwave, updater} {
		assertCAShape(t, els, icon)
	}
	assertBroadwaveTemplate(t, broadwave)
	assertUpdaterTemplate(t, updater)
}

func parseTemplate(t *testing.T, path string) []templateEl {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var els []templateEl
	var cur *templateEl
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("%s: %v", path, err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if tok.Name.Local != "Container" {
					t.Fatalf("%s: root is <%s>, want Container", path, tok.Name.Local)
				}
				version := attr(tok, "version")
				if version != "2" {
					t.Fatalf("%s: Container version %q, want 2", path, version)
				}
				continue
			}
			if depth != 2 {
				t.Fatalf("%s: nested element <%s> is not a flat template", path, tok.Name.Local)
			}
			if !caTags[tok.Name.Local] {
				t.Fatalf("%s: <%s> is not a Community Apps or dockerMan field", path, tok.Name.Local)
			}
			el := templateEl{name: tok.Name.Local, attrs: map[string]string{}}
			for _, a := range tok.Attr {
				el.attrs[a.Name.Local] = a.Value
			}
			els = append(els, el)
			cur = &els[len(els)-1]
		case xml.EndElement:
			if depth == 2 {
				cur = nil
			}
			depth--
		case xml.CharData:
			if cur != nil {
				cur.text += string(tok)
			}
		}
	}
	return els
}

func assertCAShape(t *testing.T, els []templateEl, iconPath string) {
	t.Helper()
	fields := map[string]templateEl{}
	for _, el := range els {
		if el.name != "Config" {
			fields[el.name] = el
		} else {
			assertConfig(t, el)
		}
	}
	name := strings.TrimSpace(fields["Name"].text)
	repo := strings.TrimSpace(fields["Repository"].text)
	if name == "" || repo == "" || !strings.Contains(repo, "/") {
		t.Fatalf("%s: Name and Repository are the minimum CA fields", name)
	}
	for _, key := range caRecommended {
		if strings.TrimSpace(fields[key].text) == "" {
			t.Errorf("%s: missing recommended <%s>", name, key)
		}
	}
	if !categoryRE.MatchString(strings.TrimSpace(fields["Category"].text)) {
		t.Errorf("%s: category %q", name, fields["Category"].text)
	}
	icon := strings.TrimSpace(fields["Icon"].text)
	if !httpsURL.MatchString(icon) || !strings.HasSuffix(icon, ".png") {
		t.Errorf("%s: icon %q", name, icon)
	}
	if !strings.HasSuffix(icon, "/"+iconPath) {
		t.Errorf("%s: icon does not point at %s", name, iconPath)
	}
	for _, key := range []string{"Project", "Support", "TemplateURL", "ReadMe"} {
		if !httpsURL.MatchString(strings.TrimSpace(fields[key].text)) {
			t.Errorf("%s: <%s> %q", name, key, fields[key].text)
		}
	}
	raw := "https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/unraid/"
	if !strings.HasPrefix(fields["TemplateURL"].text, raw) || !strings.HasSuffix(fields["TemplateURL"].text, ".xml") {
		t.Errorf("%s: TemplateURL %q", name, fields["TemplateURL"].text)
	}
	if strings.TrimSpace(fields["Overview"].text) == "" || len(strings.TrimSpace(fields["Overview"].text)) < 200 {
		t.Errorf("%s: Overview is too short to be the Community Apps description", name)
	}
	if strings.TrimSpace(fields["Description"].text) == "" {
		t.Errorf("%s: Description is empty", name)
	}
	blob := fields["Overview"].text + fields["Description"].text + fields["Requires"].text
	// The retired name is built here so this file does not contain it.
	retired := strings.ToLower("wave" + "guide")
	for _, banned := range []string{"192.168.", "@gmail.com", retired} {
		if strings.Contains(strings.ToLower(blob), banned) {
			t.Errorf("%s: contains %q", name, banned)
		}
	}
	if strings.TrimSpace(fields["Privileged"].text) != "false" {
		t.Errorf("%s: Privileged %q", name, fields["Privileged"].text)
	}
}

func assertConfig(t *testing.T, el templateEl) {
	t.Helper()
	for _, key := range []string{"Name", "Target", "Description", "Type", "Display", "Required", "Mask"} {
		if strings.TrimSpace(el.attrs[key]) == "" {
			t.Errorf("config %q missing %s", el.attrs["Name"], key)
		}
	}
	switch el.attrs["Type"] {
	case "Path", "Port", "Variable":
	default:
		t.Errorf("config %q type %q", el.attrs["Name"], el.attrs["Type"])
	}
	switch el.attrs["Display"] {
	case "always", "advanced":
	default:
		t.Errorf("config %q display %q", el.attrs["Name"], el.attrs["Display"])
	}
	switch el.attrs["Required"] {
	case "true", "false":
	default:
		t.Errorf("config %q required %q", el.attrs["Name"], el.attrs["Required"])
	}
	if el.attrs["Mask"] != "false" {
		t.Errorf("config %q is masked. The template has no secret to mask", el.attrs["Name"])
	}
	if strings.Contains(el.attrs["Target"], "BROADWAVE_E2E") || strings.Contains(el.attrs["Target"], "PASSWORD") || strings.Contains(el.attrs["Target"], "SECRET") {
		t.Errorf("config %q does not belong in a public template", el.attrs["Target"])
	}
}

func assertBroadwaveTemplate(t *testing.T, els []templateEl) {
	t.Helper()
	fields := indexFields(els)
	if fields["Repository"].text != "ghcr.io/wolfebase/broadwave:latest" {
		t.Errorf("repository %q", fields["Repository"].text)
	}
	if strings.TrimSpace(fields["Network"].text) != "host" {
		t.Errorf("network %q, want host", fields["Network"].text)
	}
	if fields["WebUI"].text != "http://[IP]:[PORT:8477]/" {
		t.Errorf("WebUI %q", fields["WebUI"].text)
	}
	if !strings.Contains(fields["ExtraParams"].text, "--device=/dev/dri") {
		t.Error("Extra Parameters dropped the graphics device")
	}
	for _, label := range []string{
		"com.centurylinklabs.watchtower.enable=true",
		"com.centurylinklabs.watchtower.lifecycle.pre-update=broadwave -update-check",
		"com.centurylinklabs.watchtower.lifecycle.pre-update-timeout=1",
	} {
		if !strings.Contains(fields["ExtraParams"].text, label) {
			t.Errorf("Extra Parameters missing %s", label)
		}
	}
	configs := map[string]templateEl{}
	for _, el := range els {
		if el.name == "Config" {
			configs[el.attrs["Target"]] = el
		}
	}
	for _, target := range []string{"8477", "/config", "/config/work/recordings", "HDHR_HOST", "TZ", "PUID", "PGID", "UMASK", "BROADWAVE_HOSTS", "BROADWAVE_ENCODER", "BROADWAVE_ALTERNATES"} {
		if _, ok := configs[target]; !ok {
			t.Errorf("missing config %s", target)
		}
	}
	if strings.TrimSpace(configs["8477"].text) != "8477" {
		t.Errorf("web port %q", configs["8477"].text)
	}
	if strings.TrimSpace(configs["/config/work/recordings"].text) == "" {
		t.Error("recordings host path is empty")
	}
	if _, ok := configs["BROADWAVE_RECORDINGS"]; ok {
		t.Error("BROADWAVE_RECORDINGS would send files off the recordings mount")
	}
	if strings.TrimSpace(configs["PUID"].text) != "99" || strings.TrimSpace(configs["PGID"].text) != "100" {
		t.Errorf("PUID/PGID %q %q", configs["PUID"].text, configs["PGID"].text)
	}
	if strings.TrimSpace(configs["HDHR_HOST"].text) != "" {
		t.Error("HDHR_HOST should be empty so host networking can discover the tuner")
	}
}

func assertUpdaterTemplate(t *testing.T, els []templateEl) {
	t.Helper()
	fields := indexFields(els)
	if fields["Repository"].text != "nickfedor/watchtower:1" {
		t.Errorf("updater repository %q", fields["Repository"].text)
	}
	if strings.TrimSpace(fields["Network"].text) != "bridge" {
		t.Errorf("updater network %q", fields["Network"].text)
	}
	if strings.TrimSpace(fields["Category"].text) != "Tools:Utilities" {
		t.Errorf("updater category %q", fields["Category"].text)
	}
	configs := map[string]templateEl{}
	for _, el := range els {
		if el.name == "Config" {
			configs[el.attrs["Target"]] = el
		}
	}
	for _, target := range []string{"/var/run/docker.sock", "TZ", "WATCHTOWER_SCHEDULE", "WATCHTOWER_LABEL_ENABLE", "WATCHTOWER_LIFECYCLE_HOOKS", "WATCHTOWER_CLEANUP"} {
		if _, ok := configs[target]; !ok {
			t.Errorf("updater missing %s", target)
		}
	}
	if strings.TrimSpace(configs["WATCHTOWER_SCHEDULE"].text) != "0 30 3 * * *" {
		t.Errorf("schedule %q", configs["WATCHTOWER_SCHEDULE"].text)
	}
	if strings.TrimSpace(configs["WATCHTOWER_LABEL_ENABLE"].text) != "true" || strings.TrimSpace(configs["WATCHTOWER_LIFECYCLE_HOOKS"].text) != "true" {
		t.Error("updater must stay limited to labeled containers and run the busy check")
	}
}

func indexFields(els []templateEl) map[string]templateEl {
	out := map[string]templateEl{}
	for _, el := range els {
		if el.name != "Config" {
			out[el.name] = el
		}
	}
	return out
}

func attr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
