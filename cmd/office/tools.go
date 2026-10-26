package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tung1998/agent-office/internal/clitools"
)

// office tools update: brings the AI CLIs to their latest version the way
// each was installed; a weekly script automation runs it, and a failure
// (non-zero exit) calls its agent.
func toolsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Short: "Công cụ AI CLI trên máy (Claude Code, Codex, Gemini, Antigravity)"}
	cmd.AddCommand(&cobra.Command{
		Use:     "update [tool...]",
		Short:   "Cập nhật các CLI đã cài lên bản mới nhất (mặc định: tất cả)",
		Example: "  office tools update\n  office tools update claude codex",
		RunE: func(cmd *cobra.Command, args []string) error {
			m := clitools.NewManager()
			ids := args
			if len(ids) == 0 {
				ids = m.IDs()
			}
			failed := 0
			for _, id := range ids {
				installed, before, err := m.Detect(cmd.Context(), id)
				if err != nil {
					return err
				}
				if !installed {
					if len(args) > 0 {
						fmt.Printf("%s: chưa cài\n", id)
						failed++
					}
					continue
				}
				j, err := m.Update(id)
				if errors.Is(err, clitools.ErrUnknownInstall) && len(args) == 0 {
					fmt.Printf("%s: bỏ qua, %v\n", id, err)
					continue
				}
				if err != nil {
					fmt.Printf("%s: %v\n", id, err)
					failed++
					continue
				}
				j.Wait()
				v := j.Snapshot()
				if v.State != "succeeded" {
					fmt.Printf("%s: lỗi khi chạy %s (%s)\n%s\n", id, v.Command, v.Error, v.Output)
					failed++
					continue
				}
				_, after, _ := m.Detect(cmd.Context(), id)
				if after == before {
					fmt.Printf("%s: đã mới nhất (%s)\n", id, after)
				} else {
					fmt.Printf("%s: %s → %s\n", id, before, after)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d công cụ cập nhật lỗi", failed)
			}
			return nil
		},
	})
	return cmd
}
