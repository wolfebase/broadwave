package guide

import (
	"testing"

	"broadwave/internal/store"
)

// European grabbers still write ISO-8859-1, and a guide lists a title once per
// language with the main one first.
func TestXMLTVInLatin1WithTitlesInTwoLanguages(t *testing.T) {
	doc := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<tv>" +
		"<channel id=\"one.de\"><display-name>Das Erste</display-name></channel>" +
		"<programme start=\"20261031200000 +0100\" stop=\"20261031201500 +0100\" channel=\"one.de\">" +
		"<title lang=\"de\">Tagesschau \x96 Gr\xfc\xdfe</title><title lang=\"en\">News</title>" +
		"<desc lang=\"de\">Caf\xe9 \x80 5</desc></programme></tv>"
	rows, _, err := Parse([]byte(doc), []store.Channel{{ID: 7, GuideNumber: "1", GuideName: "Das Erste"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Tagesschau – Grüße" || rows[0].Description != "Café € 5" {
		t.Fatalf("%+v", rows)
	}
	if got := rows[0].Start.UTC().Format("15:04"); got != "19:00" {
		t.Fatalf("start %s", got)
	}
	latin9 := "<?xml version=\"1.0\" encoding=\"ISO-8859-15\"?><tv><channel id=\"a\"><display-name>A</display-name></channel>" +
		"<programme start=\"20261031200000\" stop=\"20261031201500\" channel=\"a\"><title>\xa4 Show</title></programme></tv>"
	rows, _, err = Parse([]byte(latin9), []store.Channel{{ID: 1, GuideNumber: "1", GuideName: "A"}})
	if err != nil || len(rows) != 1 || rows[0].Title != "€ Show" {
		t.Fatalf("%+v %v", rows, err)
	}
	if _, _, err := Parse([]byte("<?xml version=\"1.0\" encoding=\"KOI8-R\"?><tv></tv>"), nil); err == nil {
		t.Fatal("an unknown encoding read")
	}
}
