package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/agrahamlincoln/katazuke/internal/oplog"
)

// LogCmd shows recent destructive operations.
type LogCmd struct {
	Days int `name:"days" help:"Show operations from the last N days." default:"30"`
}

// Run executes the log command.
func (c *LogCmd) Run(_ *CLI) error {
	since := time.Now().AddDate(0, 0, -c.Days)
	ops, err := oplog.ReadOps(since)
	if err != nil {
		return fmt.Errorf("reading operation log: %w", err)
	}

	if len(ops) == 0 {
		fmt.Printf("No operations in the last %d days.\n", c.Days)
		return nil
	}

	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)

	fmt.Printf("Operations from the last %d days:\n\n", c.Days)

	for _, op := range ops {
		ts := op.Timestamp.Local().Format("2006-01-02 15:04")

		switch op.Type {
		case oplog.OpDeleteBranch:
			repoName := filepath.Base(op.RepoPath)
			remoteTag := ""
			if op.DeletedRemote {
				remoteTag = " [+ remote]"
			}
			fmt.Printf("%s  %s  %s: %s%s\n",
				dim.Sprint(ts), bold.Sprint("delete_branch"), repoName, op.Branch, remoteTag)
			if op.CommitSHA != "" {
				fmt.Printf("%s  SHA: %s %s\n",
					dim.Sprint(strings.Repeat(" ", 16)),
					op.CommitSHA[:min(12, len(op.CommitSHA))],
					dim.Sprintf("(recoverable: git branch %s %s)", op.Branch, op.CommitSHA[:min(12, len(op.CommitSHA))]))
			}

		case oplog.OpDeleteRepo:
			fmt.Printf("%s  %s  %s\n",
				dim.Sprint(ts), bold.Sprint("delete_repo"), op.Path)
			if op.RemoteURL != "" {
				fmt.Printf("%s  Remote: %s\n",
					dim.Sprint(strings.Repeat(" ", 16)), op.RemoteURL)
			}

		case oplog.OpDeleteDir:
			fmt.Printf("%s  %s  %s\n",
				dim.Sprint(ts), bold.Sprint("delete_dir"), op.Path)

		case oplog.OpMoveDir:
			fmt.Printf("%s  %s  %s -> %s\n",
				dim.Sprint(ts), bold.Sprint("move_dir"), op.Path, op.Destination)

		case oplog.OpSwitchBranch:
			repoName := filepath.Base(op.RepoPath)
			fmt.Printf("%s  %s  %s: %s -> %s\n",
				dim.Sprint(ts), bold.Sprint("switch_branch"), repoName, op.PreviousBranch, op.Branch)

		default:
			fmt.Printf("%s  %s  %s\n",
				dim.Sprint(ts), bold.Sprint(string(op.Type)), op.Path)
		}
	}

	fmt.Printf("\n%d operation(s) total.\n", len(ops))
	return nil
}
