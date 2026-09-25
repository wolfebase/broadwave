package sports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppsDoNotFetchTheScoreboard(t *testing.T) {
	roots := []string{"../../../web/src", "../../../apple"}
	banned := []string{"site.api.espn.com", "thesportsdb.com", "a.espncdn.com"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == ".build" {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ts", ".tsx", ".js", ".jsx", ".swift":
			default:
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, host := range banned {
				if strings.Contains(text, host) {
					t.Errorf("%s names %s", path, host)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
