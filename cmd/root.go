// Package cmd definisce i comandi della CLI.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/rubenpari/tunescout/internal/config"
	"github.com/rubenpari/tunescout/internal/spotify"
	"github.com/rubenpari/tunescout/internal/store"
)

var (
	flagModel  string
	flagEffort string
)

var rootCmd = &cobra.Command{
	Use:   "tunescout",
	Short: "Scopri nuova musica partendo dalla tua libreria Spotify",
	Long: `tunescout legge la tua libreria Spotify (brani salvati, top artisti e brani, playlist),
chiede a Claude dei suggerimenti personalizzati e li verifica su Spotify,
escludendo quello che conosci già.

Primo utilizzo:
  tunescout login --client-id <ID app Spotify>
  tunescout sync
  tunescout discover -i`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Errore:", err)
	}
	return err
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "", "modello Claude (default: "+config.DefaultModel+", o $TUNESCOUT_MODEL)")
	rootCmd.PersistentFlags().StringVar(&flagEffort, "effort", "", "effort del modello: low|medium|high|xhigh|max (default: "+config.DefaultEffort+")")
	rootCmd.AddCommand(loginCmd, syncCmd, discoverCmd, pruneCmd, historyCmd)
}

// env raccoglie le dipendenze comuni ai comandi.
type env struct {
	cfg *config.Config
	st  *store.Store
	sp  *spotify.Client
}

func setup(ctx context.Context, needSpotify bool) (*env, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	cfg = cfg.WithDefaults()
	if flagModel != "" {
		cfg.Model = flagModel
	}
	if flagEffort != "" {
		cfg.Effort = flagEffort
	}
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("apertura database: %w", err)
	}
	e := &env{cfg: cfg, st: st}
	if needSpotify {
		if cfg.SpotifyClientID == "" {
			st.Close()
			return nil, errors.New("client ID Spotify mancante: esegui `tunescout login --client-id <ID>`")
		}
		if e.sp, err = spotify.New(ctx, cfg.SpotifyClientID); err != nil {
			st.Close()
			return nil, err
		}
	}
	return e, nil
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }
