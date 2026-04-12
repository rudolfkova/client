package characterweb

import (
	"os"
	"path/filepath"
	"sort"
)

// ListAnimSpriteIDs возвращает имена наборов: для каждой подпапки dataRoot/anim/<name>,
// если существует файл dataRoot/anim/<name>/<name>.png — id в список.
func ListAnimSpriteIDs(dataRoot string) ([]string, error) {
	animDir := filepath.Join(dataRoot, "anim")
	entries, err := os.ReadDir(animDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		pngPath := filepath.Join(animDir, name, name+".png")
		st, err := os.Stat(pngPath)
		if err != nil || st.IsDir() {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}
