package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

const (
	rootDir    = "files"
	tmplSuffix = ".gotmpl"
	skipMarker = "{{/* SKIP */}}"
)

type Context struct {
	ModulePath string
	AppName    string
	DBName     string
	Timezone   string
	Features   Features
}

type Features struct {
	Register       bool
	Login          bool
	ForgotPassword bool
	RefreshToken   bool
	Email          bool
}

// Render walks the embedded template tree and writes rendered files to dst.
// Directories and files are created relative to dst; .tmpl suffix is stripped.
func Render(dst string, ctx Context) error {
	funcs := sprig.TxtFuncMap()
	funcs["hasFeature"] = func(name string) bool {
		switch strings.ToLower(name) {
		case "register":
			return ctx.Features.Register
		case "login":
			return ctx.Features.Login
		case "forgot", "forgot_password":
			return ctx.Features.ForgotPassword
		case "refresh", "refresh_token":
			return ctx.Features.RefreshToken
		case "email":
			return ctx.Features.Email
		}
		return false
	}

	return fs.WalkDir(FS, rootDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if srcPath == rootDir {
			return nil
		}

		rel := strings.TrimPrefix(srcPath, rootDir+"/")
		rel = resolveFilename(rel)
		targetPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		raw, readErr := fs.ReadFile(FS, srcPath)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", srcPath, readErr)
		}

		if !strings.HasSuffix(srcPath, tmplSuffix) {
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			return os.WriteFile(targetPath, raw, 0o644)
		}

		tmpl, parseErr := template.New(srcPath).Funcs(funcs).Parse(string(raw))
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", srcPath, parseErr)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return fmt.Errorf("execute %s: %w", srcPath, err)
		}

		rendered := buf.Bytes()
		if bytes.HasPrefix(bytes.TrimSpace(rendered), []byte(skipMarker)) {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, rendered, 0o644)
	})
}

// resolveFilename strips .tmpl suffix and maps placeholder names that embed
// cannot carry literally (e.g. dotfiles inside //go:embed).
func resolveFilename(rel string) string {
	rel = strings.TrimSuffix(rel, tmplSuffix)
	rel = strings.ReplaceAll(rel, "_dot_", ".")
	return rel
}
