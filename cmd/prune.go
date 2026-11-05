package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rubenpari/tunescout/internal/llm"
	"github.com/rubenpari/tunescout/internal/norm"
	"github.com/rubenpari/tunescout/internal/prune"
	"github.com/rubenpari/tunescout/internal/store"
)

var (
	flagPruneCount    int
	flagPruneMinAge   int
	flagPrunePlaylist string
	flagPruneRemove   bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Suggerisce brani da eliminare dalla libreria perché poco ascoltati e lontani dai tuoi gusti",
	Long: `prune cerca tra i brani salvati quelli che ascolti poco e chiede a Claude quali sono
meno in linea con il tuo profilo.

Spotify non fornisce il numero di ascolti: un brano è considerato poco ascoltato se è
salvato da almeno --min-age giorni, non compare tra i tuoi top brani (4 settimane,
6 mesi, 1 anno) e nessuno dei suoi artisti è tra quelli che ascolti di più.

Senza --remove il comando non elimina nulla. Con --playlist raccoglie i brani in una
playlist privata, così puoi riascoltarli e decidere. Con --remove, dopo l'elenco, chiede
conferma e li rimuove dai "Brani che ti piacciono" (usato con --playlist, la playlist
resta come copia di sicurezza). I brani rimossi non verranno più proposti da discover.`,
	Example: `  tunescout prune
  tunescout prune -n 40 --min-age 365
  tunescout prune --playlist "Da eliminare"
  tunescout prune --remove --playlist "Rimossi da tunescout"`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		if flagPruneCount < 1 || flagPruneCount > 50 {
			return fmt.Errorf("-n deve essere tra 1 e 50")
		}
		if flagPruneMinAge < 0 {
			return fmt.Errorf("--min-age non può essere negativo")
		}
		if flagPruneRemove && !term.IsTerminal(os.Stdin.Fd()) {
			return errors.New("--remove richiede un terminale interattivo per la conferma")
		}
		e, err := setup(ctx, true)
		if err != nil {
			return err
		}
		defer e.st.Close()
		if err := ensureSynced(ctx, e); err != nil {
			return err
		}

		p := &prune.Pipeline{SP: e.sp, LLM: llm.New(e.cfg.Model, e.cfg.Effort), ST: e.st, Log: logf}
		res, stats, err := p.Run(ctx, prune.Options{
			Count: flagPruneCount, MinAge: time.Duration(flagPruneMinAge) * 24 * time.Hour,
		})
		if err != nil {
			return err
		}
		logf("%s", stats)
		if len(res) == 0 {
			fmt.Println("Nessun brano da eliminare: la tua libreria è in linea con i tuoi gusti.")
			return nil
		}

		list := make([]store.Suggestion, len(res))
		notes := make([]string, len(res))
		for i, r := range res {
			t := r.Track
			list[i] = store.Suggestion{
				Key: norm.Key(t.Artists[0].Name, t.Name), SpotifyID: t.ID, URI: t.URI,
				Artist: strings.Join(t.ArtistNames(), ", "), Title: t.Name,
				Album: t.Album.Name, Year: t.Year(), Reason: r.Reason, URL: t.URL(),
			}
			notes[i] = "salvato il " + r.AddedAt.Format("02/01/2006")
		}

		if flagPrunePlaylist != "" {
			pl, err := createPlaylist(ctx, e.sp, flagPrunePlaylist, "Brani da valutare per l'eliminazione, scelti da tunescout", list)
			if err != nil {
				return err
			}
			logf("✓ Playlist %q creata con %d brani: %s", pl.Name, len(list), pl.URL)
		}
		printTable(list, notes)

		if flagPruneRemove {
			fmt.Println()
			ok, err := confirm(os.Stdin, fmt.Sprintf("Rimuovere questi %d brani dalla libreria? [s/N] ", len(list)))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Nessun brano rimosso.")
				return nil
			}
			// Registra prima di rimuovere: se la rimozione fallisce a metà, i brani
			// rimasti in libreria sarebbero comunque esclusi da discover.
			removed := make([]store.KnownTrack, len(list))
			for i, s := range list {
				removed[i] = store.KnownTrack{Key: s.Key, SpotifyID: s.SpotifyID, Artist: s.Artist, Title: s.Title, Source: "removed"}
			}
			if err := e.st.AddRemoved(removed); err != nil {
				return fmt.Errorf("registrazione dei brani rimossi: %w", err)
			}
			if err := e.sp.RemoveFromLibrary(ctx, suggestionURIs(list)); err != nil {
				return fmt.Errorf("rimozione dalla libreria: %w", err)
			}
			fmt.Printf("✓ %d brani rimossi dalla libreria.\n", len(list))
		}
		return nil
	},
}

// confirm mostra la domanda e accetta solo una risposta affermativa esplicita.
func confirm(r io.Reader, question string) (bool, error) {
	fmt.Print(question)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "s", "si", "sì", "y", "yes":
		return true, nil
	}
	return false, nil
}

func init() {
	f := pruneCmd.Flags()
	f.IntVarP(&flagPruneCount, "count", "n", 20, "numero massimo di brani da proporre (1-50)")
	f.IntVar(&flagPruneMinAge, "min-age", 90, "considera solo i brani salvati da almeno questi giorni")
	f.StringVar(&flagPrunePlaylist, "playlist", "", "raccoglie i brani proposti in una playlist privata con questo nome")
	f.BoolVar(&flagPruneRemove, "remove", false, "dopo l'elenco chiede conferma e rimuove i brani dalla libreria")
	f.BoolVar(&flagNoSync, "no-sync", false, "non aggiornare automaticamente la cache della libreria")
}
