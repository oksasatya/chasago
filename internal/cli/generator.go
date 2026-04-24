package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/oksasatya/chasago/internal/template"
)

// Generate renders the embedded template tree into dst with the provided context,
// then runs `go mod tidy` and `git init` in dst (best-effort — not fatal).
func Generate(dst string, ctx template.Context, out io.Writer) error {
	if err := template.Render(dst, ctx); err != nil {
		return fmt.Errorf("render templates: %w", err)
	}

	fmt.Fprintln(out, "→ running go mod tidy")
	if err := runIn(dst, out, "go", "mod", "tidy"); err != nil {
		fmt.Fprintf(out, "  warning: go mod tidy failed: %v\n", err)
	}

	if !gitInitialized(dst) {
		fmt.Fprintln(out, "→ initializing git repo")
		if err := runIn(dst, out, "git", "init", "-q"); err != nil {
			fmt.Fprintf(out, "  warning: git init failed: %v\n", err)
		}
	}

	return nil
}

func runIn(dir string, out io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func gitInitialized(dir string) bool {
	info, err := os.Stat(dir + "/.git")
	return err == nil && info.IsDir()
}
