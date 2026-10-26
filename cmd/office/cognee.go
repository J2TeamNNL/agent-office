package main

import (
	"cmp"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tung1998/agent-office/internal/cognee"
)

// office cognee: office runs Cognee in Docker, so recall finds the agents'
// notes by meaning (ADR-195). A running office picks it up within 2 minutes.
func cogneeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "cognee", Short: "Cognee chạy trong Docker: tìm ghi chú của agent theo nghĩa"}
	var port int
	up := &cobra.Command{
		Use:   "up",
		Short: "Bật: chạy container Cognee, office tự khởi động nó từ nay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withApp(cmd.Context(), func(a *app) error {
				s := cognee.Load(cmd.Context(), a.store)
				if port > 0 {
					s.Port = port
				}
				s.Docker = true
				fmt.Printf("Chạy %s (%s) ở 127.0.0.1:%d; lần đầu tải model, có thể vài phút…\n", cognee.Container, cognee.Image, s.Port)
				if err := cognee.Up(cmd.Context(), s.Port); err != nil {
					return err
				}
				if err := cognee.Save(cmd.Context(), a.store, s); err != nil {
					return err
				}
				fmt.Println("Cognee đã chạy. Office đang chạy sẽ đồng bộ ghi chú trong vòng 2 phút.")
				return nil
			})
		},
	}
	up.Flags().IntVar(&port, "port", 0, fmt.Sprintf("cổng trên máy (mặc định %d)", cognee.DefaultPort))
	cmd.AddCommand(up, &cobra.Command{
		Use:   "down",
		Short: "Tắt: xóa container (giữ dữ liệu trong volume), office không chạy nó nữa",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withApp(cmd.Context(), func(a *app) error {
				s := cognee.Load(cmd.Context(), a.store)
				s.Docker = false
				if err := cognee.Save(cmd.Context(), a.store, s); err != nil {
					return err
				}
				return cognee.Down(cmd.Context())
			})
		},
	}, &cobra.Command{
		Use:   "status",
		Short: "Container, cổng và Cognee có trả lời không",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withApp(cmd.Context(), func(a *app) error {
				s := cognee.Load(cmd.Context(), a.store)
				state, ok := cognee.Status(cmd.Context(), s.Port)
				fmt.Printf("bật: %v, cổng: %d, container: %s, trả lời: %v\n", s.Docker, s.Port, cmp.Or(state, "không có"), ok)
				return nil
			})
		},
	})
	return cmd
}
