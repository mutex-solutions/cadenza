// Command cadenza-gen writes the files generated from internal/spec:
//
//	web/packages/core/src/api.gen.ts  TypeScript types and endpoint table
//	internal/guide/cadenza.md         the agent guide (embedded in the CLI)
//	docs/cadenza.md                   the same guide, for reading on GitHub
//
// Run it with `make gen`. CI fails if the committed files are stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	files := map[string]string{
		"web/packages/core/src/api.gen.ts": TypeScript(),
		"internal/guide/cadenza.md":        Guide(),
		"docs/cadenza.md":                  Guide(),
	}
	for path, body := range files {
		full := filepath.Join(*root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "cadenza-gen:", err)
	os.Exit(1)
}
