// Package main provides the katazuke CLI tool for workspace maintenance.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "katazuke",
	Short: "Developer workspace maintenance tool",
	Long: `katazuke (片付け) - "tidying up"

A developer workspace maintenance tool that helps you keep your ~/projects
directory clean and organized by managing stale branches, archived repositories,
and out-of-date checkouts.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

func init() {
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(branchesCmd)
	rootCmd.AddCommand(reposCmd)
	rootCmd.AddCommand(syncCmd)

	// Global flags
	rootCmd.PersistentFlags().BoolP("dry-run", "n", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().StringP("projects-dir", "p", "", "Projects directory (default: ~/projects)")
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run full workspace audit",
	Long:  "Scan the projects directory and report on all potential cleanup opportunities",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Running workspace audit...")
		fmt.Println("(Implementation coming soon)")
	},
}

var branchesCmd = &cobra.Command{
	Use:   "branches",
	Short: "Manage branches across repositories",
	Long:  "Find and clean up merged or stale branches",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Analyzing branches...")
		fmt.Println("(Implementation coming soon)")
	},
}

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Manage repository checkouts",
	Long:  "Find and remove archived or defunct repository checkouts",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Analyzing repositories...")
		fmt.Println("(Implementation coming soon)")
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all repositories",
	Long:  "Update all repositories by pulling latest changes from remotes",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Syncing repositories...")
		fmt.Println("(Implementation coming soon)")
	},
}

func init() {
	// Branch command flags
	branchesCmd.Flags().Bool("merged", false, "Show only merged branches")
	branchesCmd.Flags().Bool("stale", false, "Show only stale branches")
	branchesCmd.Flags().Int("stale-days", 30, "Days before a branch is considered stale")

	// Repos command flags
	reposCmd.Flags().Bool("archived", false, "Show only archived repositories")
	reposCmd.Flags().Bool("backup", true, "Create backup before deletion")

	// Audit command flags
	auditCmd.Flags().Bool("non-git", false, "Show only non-git directories")
}
