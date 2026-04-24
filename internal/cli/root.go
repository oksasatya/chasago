package cli

import (
	"github.com/spf13/cobra"

	"github.com/oksasatya/chasago/internal/version"
)
 
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "chasago",
		Short: "Boilerplate generator untuk Go REST API (clean architecture + fx)",
		Long: "chasago adalah CLI yang men-generate struktur project REST API Go " +
			"siap pakai: Gin + uber-fx + sqlx/pgx + Paseto + Redis + golang-migrate.",
		Version:      version.String(),
		SilenceUsage: true,
	}
	root.SetVersionTemplate("chasago {{.Version}}\n")
	root.AddCommand(newInitCmd())
	return root
}
