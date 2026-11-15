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
	"ExtraSearchTerms": true, "Screenshot": true,
}

// The Community Apps feed reads templates from their own repository; deploy/unraid/ca is
// that repository's layout, and its template differs from deploy/unraid/broadwave.xml only
// in TemplateURL.
const (
	rawRepo     = "https://raw.githubusercontent.com/wolfebase/broadwave/main/"
	rawTemplate = "https://raw.githubusercontent.com/wolfebase/unraid-templates/main/"
)

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
	listed := parseTemplate(t, filepath.Join(root, "deploy/unraid/ca/templates/broadwave.xml"))
	assertCAShape(t, broadwave, icon, rawRepo+"deploy/unraid/broadwave.xml")
	assertCAShape(t, updater, icon, rawRepo+"deploy/unraid/broadwave-updater.xml")
	assertCAShape(t, listed, icon, rawTemplate+"templates/broadwave.xml")
	assertBroadwaveTemplate(t, broadwave)
	assertBroadwaveTemplate(t, listed)
	assertUpdaterTemplate(t, updater)
	assertScreenshots(t, root, broadwave)
}

func TestCommunityAppsRepository(t *testing.T) {
	root := filepath.Dir(docsDir(t))
	ca := filepath.Join(root, "deploy/unraid/ca")
	ours, err := os.ReadFile(filepath.Join(root, "deploy/unraid/broadwave.xml"))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := os.ReadFile(filepath.Join(ca, "templates/broadwave.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(ours), rawRepo+"deploy/unraid/broadwave.xml", rawTemplate+"templates/broadwave.xml", 1)
	if string(listed) != want {
		t.Error("deploy/unraid/ca/templates/broadwave.xml must be deploy/unraid/broadwave.xml with only TemplateURL changed. Copy it again.")
	}
	license, err := os.ReadFile(filepath.Join(ca, "LICENSE"))
	if err != nil || !strings.Contains(string(license), "Apache License") {
		t.Errorf("the template repository needs an OSI license at its root: %v", err)
	}
	f, err := os.Open(filepath.Join(ca, "ca_profile.xml"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var profile struct {
		XMLName xml.Name `xml:"CommunityApplications"`
		Profile string   `xml:"Profile"`
		Icon    string   `xml:"Icon"`
		WebPage string   `xml:"WebPage"`
	}
	if err := xml.NewDecoder(f).Decode(&profile); err != nil {
		t.Fatalf("ca_profile.xml: %v", err)
	}
	if strings.TrimSpace(profile.Profile) == "" {
		t.Error("ca_profile.xml needs a non-empty <Profile>; the submission is blocked without it")
	}
	for _, u := range []string{profile.Icon, profile.WebPage} {
		if !httpsURL.MatchString(strings.TrimSpace(u)) {
			t.Errorf("ca_profile.xml URL %q", u)
		}
	}
	// The submission flow wants a repository icon at the root.
	icon, err := os.ReadFile(filepath.Join(ca, "icon.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(icon, new(struct {
		XMLName xml.Name `xml:"svg"`
	})); err != nil {
		t.Errorf("icon.svg: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(ca, "templates"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the listed repository carries one template, broadwave.xml; found %d files", len(entries))
	}
}

// assertScreenshots checks that each Screenshot is a raw URL for a file in this repository.
func assertScreenshots(t *testing.T, root string, els []templateEl) {
	t.Helper()
	n := 0
	for _, el := range els {
		if el.name != "Screenshot" {
			continue
		}
		n++
		u := strings.TrimSpace(el.text)
		if !strings.HasPrefix(u, rawRepo) {
			t.Errorf("screenshot %q is not a raw URL in this repository", u)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(u, rawRepo)))); err != nil {
			t.Errorf("screenshot %q: %v", u, err)
		}
	}
	if n == 0 {
		t.Error("no screenshots for the Community Apps page")
	}
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

func assertCAShape(t *testing.T, els []templateEl, iconPath, templateURL string) {
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
	if fields["TemplateURL"].text != templateURL {
		t.Errorf("%s: TemplateURL %q, want %q (the raw URL of that exact file)", name, fields["TemplateURL"].text, templateURL)
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
	case "Path", "Port", "Variable", "Device":
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
	// The graphics device is its own entry, so a server without one can remove it in the form.
	// In Extra Parameters too, Docker would get it twice.
	if strings.Contains(fields["ExtraParams"].text, "/dev/dri") {
		t.Error("/dev/dri belongs in its Device entry, not Extra Parameters")
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
	if dri := configs["/dev/dri"]; dri.attrs["Type"] != "Device" || strings.TrimSpace(dri.text) != "/dev/dri" || dri.attrs["Required"] != "false" {
		t.Errorf("graphics entry %+v, want an optional Device entry for /dev/dri", dri.attrs)
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
