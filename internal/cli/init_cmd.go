package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oksasatya/chasago/internal/template"
)

func newInitCmd() *cobra.Command {
	var (
		skipPrompts bool
		modulePath  string
		appName     string
		dbName      string
		timezone    string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate boilerplate di folder saat ini (in-place)",
		Long: "Generate project Go REST API (clean architecture, gin, uber-fx, paseto, redis, golang-migrate) " +
			"di direktori kerja saat ini. Jalankan di dalam folder project kosong.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			var ans Answers
			if skipPrompts {
				ans = defaultAnswers(cwd)
			} else {
				ok, err := ConfirmOverwrite(cwd)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "dibatalkan.")
					return nil
				}
				ans, err = Ask(cwd)
				if err != nil {
					return err
				}
			}

			if modulePath != "" {
				ans.ModulePath = modulePath
			}
			if appName != "" {
				ans.AppName = appName
			}
			if dbName != "" {
				ans.DBName = dbName
			}
			if timezone != "" {
				ans.Timezone = timezone
			}

			if err := Generate(cwd, ans.ToContext(), out); err != nil {
				return err
			}

			printNextSteps(out, ans.AppName)
			return nil
		},
	}

	cmd.Flags().BoolVar(&skipPrompts, "yes", false, "pakai jawaban default (non-interaktif)")
	cmd.Flags().StringVar(&modulePath, "module", "", "override module path")
	cmd.Flags().StringVar(&appName, "app", "", "override app name")
	cmd.Flags().StringVar(&dbName, "db", "", "override database name")
	cmd.Flags().StringVar(&timezone, "timezone", "", "override timezone (default Asia/Jakarta)")
	return cmd
}

func defaultAnswers(cwd string) Answers {
	app := strings.ToLower(filepath.Base(cwd))
	return Answers{
		ModulePath: "github.com/your-org/" + app,
		AppName:    app,
		DBName:     strings.ReplaceAll(app, "-", "_"),
		Timezone:   "Asia/Jakarta",
		Features: template.Features{
			Register:       true,
			Login:          true,
			ForgotPassword: true,
			RefreshToken:   true,
			Email:          true,
		},
	}
}

func printNextSteps(w io.Writer, appName string) {
	msg := "\n" +
		"✓ project " + appName + " siap.\n\n" +
		"Next steps:\n" +
		"  1. cp .env.example .env\n" +
		"  2. edit .env (DB_PASSWORD, PASETO_SYMMETRIC_KEY, SMTP_*, FRONTEND_URL)\n" +
		"  3. docker compose up -d postgres redis\n" +
		"  4. make migrate-up\n" +
		"  5. make seed              # super-admin + 50 dummy users\n" +
		"  6. make dev\n\n" +
		"Scaffold helpers:\n" +
		"  make seeder name=Post     # internal/database/seeders/post_seeder.go\n" +
		"  make factory name=Post    # internal/database/factories/post_factory.go\n\n" +
		"Default admin (ganti setelah login pertama):\n" +
		"  email:    admin@local\n" +
		"  password: Admin@123\n"
	_, _ = w.Write([]byte(msg))
}
