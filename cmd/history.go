package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	flagHistLimit    int
	flagHistFeedback string
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Mostra i suggerimenti passati e il tuo feedback",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if f := flagHistFeedback; f != "" && f != "like" && f != "skip" {
			return fmt.Errorf("--feedback deve essere like o skip")
		}
		e, err := setup(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer e.st.Close()
		list, err := e.st.History(flagHistLimit, flagHistFeedback)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("Nessun suggerimento in archivio.")
			return nil
		}
		for _, s := range list {
			mark := " "
			switch s.Feedback {
			case "like":
				mark = "♥"
			case "skip":
				mark = "✗"
			}
			fmt.Printf("%s %s  %s — %s\n", mark, s.CreatedAt.Format("2006-01-02"), s.Artist, s.Title)
		}
		return nil
	},
}

func init() {
	historyCmd.Flags().IntVarP(&flagHistLimit, "limit", "n", 50, "numero massimo di voci (0 = tutte)")
	historyCmd.Flags().StringVar(&flagHistFeedback, "feedback", "", "filtra per feedback: like|skip")
}
