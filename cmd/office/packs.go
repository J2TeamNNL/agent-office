package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tung1998/agent-office/templates/packs"
)

// packsCmd lists office's library of project packs (ADR-199).
func packsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "packs",
		Short: "Thư viện mẫu project của office: tên để dùng với init --template",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := packs.Bundles()
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			for _, b := range list {
				fmt.Fprintf(tw, "%s\t%s\n", b.Name, b.Title)
				for _, e := range b.Packs {
					fmt.Fprintf(tw, "  %s\t%s: %s\n", e.Ref, e.Title, e.Description)
				}
			}
			return tw.Flush()
		},
	}
}
