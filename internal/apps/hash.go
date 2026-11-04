package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"easy42/internal/config"
)

// ComputeAppHash returns a deterministic SHA-256 hash of an App definition
func ComputeAppHash(app config.App) string {
	h := sha256.New()
	fmt.Fprintf(h, "name:%s\n", app.Name)
	fmt.Fprintf(h, "type:%s\n", app.AppType)
	fmt.Fprintf(h, "enabled:%t\n", app.Enabled)
	fmt.Fprintf(h, "main:%s\n", app.MainContent)
	fmt.Fprintf(h, "env:%s\n", app.EnvContent)

	if len(app.Files) > 0 {
		var keys []string
		for k := range app.Files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "file:%s:%s\n", k, app.Files[k])
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}
