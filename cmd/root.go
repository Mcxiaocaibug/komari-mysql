package cmd

import (
	"fmt"
	"os"

	"github.com/komari-monitor/komari/cmd/flags"

	"github.com/spf13/cobra"
)

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

var RootCmd = &cobra.Command{
	Use:   "Komari",
	Short: "Komari is a simple server monitoring tool",
	Long: `Komari is a simple server monitoring tool. 
Made by Akizon77 with love.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SetArgs([]string{"server"})
		cmd.Execute()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&flags.DatabaseType, "db-type", "t", GetEnv("KOMARI_DB_TYPE", "sqlite"), "Main database type (sqlite, mysql) [env: KOMARI_DB_TYPE]")
	RootCmd.PersistentFlags().StringVarP(&flags.DatabaseFile, "database", "d", GetEnv("KOMARI_DB_FILE", "./data/komari.db"), "SQLite database file path [env: KOMARI_DB_FILE]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseDSN, "db-dsn", GetEnv("KOMARI_DB_DSN", ""), "MySQL DSN; takes precedence over split options [env: KOMARI_DB_DSN]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseHost, "db-host", GetEnv("KOMARI_DB_HOST", "localhost"), "MySQL host [env: KOMARI_DB_HOST]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePort, "db-port", GetEnv("KOMARI_DB_PORT", "3306"), "MySQL port [env: KOMARI_DB_PORT]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseUser, "db-user", GetEnv("KOMARI_DB_USER", "root"), "MySQL username [env: KOMARI_DB_USER]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePass, "db-pass", GetEnv("KOMARI_DB_PASS", ""), "MySQL password [env: KOMARI_DB_PASS]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseName, "db-name", GetEnv("KOMARI_DB_NAME", "komari"), "MySQL database name [env: KOMARI_DB_NAME]")
}
