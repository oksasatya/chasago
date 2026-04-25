package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/oksasatya/chasago/internal/template"
)

// detectGithubUser tries gh CLI first, then env vars. Returns empty when no
// reliable signal — caller should let the user fill manually instead of
// guessing.
func detectGithubUser() string {
	if out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output(); err == nil {
		if u := strings.TrimSpace(string(out)); u != "" {
			return u
		}
	}
	for _, key := range []string{"GITHUB_USERNAME", "GITHUB_USER", "GH_USER"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// detectTimezone resolves the host timezone — $TZ first, then the symlink
// at /etc/localtime (Linux/macOS), then a sane fallback.
func detectTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		return tz
	}
	if dest, err := os.Readlink("/etc/localtime"); err == nil {
		// e.g. /var/db/timezone/zoneinfo/Asia/Jakarta -> "Asia/Jakarta"
		const marker = "/zoneinfo/"
		if i := strings.Index(dest, marker); i != -1 {
			if tz := dest[i+len(marker):]; tz != "" {
				return tz
			}
		}
	}
	return "Asia/Jakarta"
}

// defaultModulePath builds a sensible module path default. Falls back to
// the legacy `your-org` placeholder only when we can't detect anything.
func defaultModulePath(appName string) string {
	if user := detectGithubUser(); user != "" {
		return "github.com/" + user + "/" + appName
	}
	return "github.com/your-org/" + appName
}

func modulePathValidator(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("module path tidak boleh kosong")
	}
	if strings.Contains(s, "your-org") {
		return fmt.Errorf("ganti 'your-org' dengan username/organisasi GitHub kamu")
	}
	if !strings.Contains(s, "/") {
		return fmt.Errorf("module path harus pakai format domain/owner/repo, mis. github.com/oksasatya/myapp")
	}
	return nil
}

type Answers struct {
	ModulePath string
	AppName    string
	DBName     string
	Timezone   string
	Features   template.Features
}

func (a Answers) ToContext() template.Context {
	return template.Context{
		ModulePath: a.ModulePath,
		AppName:    a.AppName,
		DBName:     a.DBName,
		Timezone:   a.Timezone,
		Features:   a.Features,
	}
}

func Ask(cwd string) (Answers, error) {
	defaultApp := strings.ToLower(filepath.Base(cwd))
	defaultModule := defaultModulePath(defaultApp)
	defaultDB := strings.ReplaceAll(defaultApp, "-", "_")

	ans := Answers{
		ModulePath: defaultModule,
		AppName:    defaultApp,
		DBName:     defaultDB,
		Timezone:   detectTimezone(),
		Features: template.Features{
			Register:       true,
			Login:          true,
			ForgotPassword: true,
			RefreshToken:   true,
			Email:          true,
		},
	}

	featureOpts := []huh.Option[string]{
		huh.NewOption("register", "register").Selected(true),
		huh.NewOption("login", "login").Selected(true),
		huh.NewOption("forgot password", "forgot").Selected(true),
		huh.NewOption("refresh token", "refresh").Selected(true),
	}
	var features []string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Go module path").
				Description("Dipakai di go.mod & semua import. Contoh: github.com/oksasatya/myapp").
				Value(&ans.ModulePath).
				Validate(modulePathValidator),
			huh.NewInput().
				Title("App name").
				Description("Dipakai di docker-compose, DB, logs").
				Value(&ans.AppName).
				Validate(requiredValidator("app name")),
			huh.NewInput().
				Title("Database name").
				Value(&ans.DBName).
				Validate(requiredValidator("database name")),
			huh.NewInput().
				Title("Default timezone").
				Value(&ans.Timezone).
				Validate(requiredValidator("timezone")),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Fitur auth yang di-include").
				Options(featureOpts...).
				Value(&features),
			huh.NewConfirm().
				Title("Enable email (SMTP + reset password)?").
				Value(&ans.Features.Email),
		),
	)

	if err := form.Run(); err != nil {
		return ans, err
	}

	ans.Features.Register = contains(features, "register")
	ans.Features.Login = contains(features, "login")
	ans.Features.ForgotPassword = contains(features, "forgot")
	ans.Features.RefreshToken = contains(features, "refresh")
	return ans, nil
}

func requiredValidator(name string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s tidak boleh kosong", name)
		}
		return nil
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ConfirmOverwrite prompts when the target dir is not empty.
func ConfirmOverwrite(cwd string) (bool, error) {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return false, err
	}

	blocking := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == ".idea" || name == ".vscode" || name == ".DS_Store" {
			continue
		}
		blocking = append(blocking, name)
	}
	if len(blocking) == 0 {
		return true, nil
	}

	var ok bool
	err = huh.NewConfirm().
		Title("Folder tidak kosong").
		Description("Ditemukan: " + strings.Join(blocking, ", ") + ".\nLanjut generate di sini?").
		Affirmative("Lanjut").
		Negative("Batal").
		Value(&ok).
		Run()
	return ok, err
}
