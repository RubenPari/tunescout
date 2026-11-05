package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rubenpari/tunescout/internal/profile"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Scarica la tua libreria Spotify nella cache locale",
	RunE: func(cmd *cobra.Command, _ []string) error {
		e, err := setup(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer e.st.Close()
		if err := profile.Sync(cmd.Context(), e.sp, e.st, logf); err != nil {
			return err
		}
		tracks, artists, err := e.st.CountKnown()
		if err != nil {
			return err
		}
		fmt.Printf("✓ Libreria sincronizzata: %d brani noti, %d artisti.\n", tracks, artists)
		return nil
	},
}
