package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rubenpari/tunescout/internal/config"
	"github.com/rubenpari/tunescout/internal/spotify"
)

var flagClientID string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Autorizza tunescout ad accedere al tuo account Spotify",
	Long: `Esegue il login OAuth (PKCE, senza client secret).

Serve un'app registrata su https://developer.spotify.com/dashboard con:
  - Redirect URI: ` + config.RedirectURL + `
  - API: Web API
  - il tuo account aggiunto in "User Management" (Development Mode)`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if flagClientID != "" {
			cfg.SpotifyClientID = flagClientID
		}
		if cfg.SpotifyClientID == "" {
			return errors.New("specifica --client-id (o la variabile SPOTIFY_CLIENT_ID)")
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		if err := spotify.Login(cmd.Context(), cfg.SpotifyClientID); err != nil {
			return err
		}
		sp, err := spotify.New(cmd.Context(), cfg.SpotifyClientID)
		if err != nil {
			return err
		}
		me, err := sp.Me(cmd.Context())
		if err != nil {
			return fmt.Errorf("login riuscito ma lettura profilo fallita: %w", err)
		}
		name := me.DisplayName
		if name == "" {
			name = me.ID
		}
		fmt.Printf("✓ Connesso come %s. Ora esegui `tunescout sync`.\n", name)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&flagClientID, "client-id", "", "Client ID dell'app Spotify")
}
