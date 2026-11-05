package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rubenpari/tunescout/internal/discover"
	"github.com/rubenpari/tunescout/internal/llm"
	"github.com/rubenpari/tunescout/internal/profile"
	"github.com/rubenpari/tunescout/internal/spotify"
	"github.com/rubenpari/tunescout/internal/store"
	"github.com/rubenpari/tunescout/internal/tui"
)

var (
	flagCount       int
	flagMood        string
	flagGenres      []string
	flagNewArtists  bool
	flagPlaylist    string
	flagInteractive bool
	flagNoSync      bool
	flagSave        bool
)

const defaultInteractivePlaylist = "tunescout · scoperte"

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Suggerisce brani nuovi in base ai tuoi gusti",
	Example: `  tunescout discover
  tunescout discover -n 30 --mood "malinconico, notturno"
  tunescout discover --genre shoegaze --genre dream-pop --new-artists
  tunescout discover --playlist "Scoperte di novembre"
  tunescout discover --save
  tunescout discover -i`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		if flagCount < 1 || flagCount > 50 {
			return fmt.Errorf("-n deve essere tra 1 e 50")
		}
		e, err := setup(ctx, true)
		if err != nil {
			return err
		}
		defer e.st.Close()

		if err := ensureSynced(ctx, e); err != nil {
			return err
		}

		p := &discover.Pipeline{SP: e.sp, LLM: llm.New(e.cfg.Model, e.cfg.Effort), ST: e.st, Log: logf}
		res, stats, err := p.Run(ctx, discover.Options{
			Count: flagCount, Mood: flagMood, Genres: flagGenres, NewArtistsOnly: flagNewArtists,
		})
		if err != nil {
			return err
		}
		logf("%s", stats)
		if len(res) == 0 {
			fmt.Println("Nessun brano nuovo trovato: prova con meno vincoli o riesegui.")
			return nil
		}
		if len(res) < flagCount {
			logf("Trovati %d brani validi su %d richiesti.", len(res), flagCount)
		}

		if flagPlaylist != "" && !flagInteractive {
			pl, err := createPlaylist(ctx, e.sp, flagPlaylist, "Suggerimenti generati da tunescout", res)
			if err != nil {
				return err
			}
			logf("✓ Playlist %q creata con %d brani: %s", pl.Name, len(res), pl.URL)
		}

		if flagSave {
			if err := e.sp.SaveToLibrary(ctx, suggestionURIs(res)); err != nil {
				return fmt.Errorf("salvataggio in libreria: %w", err)
			}
			for _, r := range res {
				if err := e.st.SetFeedback(r.Key, "like"); err != nil {
					return err
				}
			}
			logf("✓ %d brani salvati nella libreria", len(res))
		}

		if flagInteractive {
			name := flagPlaylist
			if name == "" {
				name = defaultInteractivePlaylist
			}
			return tui.Run(ctx, res, &actions{sp: e.sp, st: e.st, playlistName: name})
		}
		printTable(res, nil)
		return nil
	},
}

func init() {
	f := discoverCmd.Flags()
	f.IntVarP(&flagCount, "count", "n", 20, "numero di brani da suggerire (1-50)")
	f.StringVar(&flagMood, "mood", "", "atmosfera desiderata, es. \"energico\" o \"per studiare\"")
	f.StringSliceVar(&flagGenres, "genre", nil, "genere desiderato (ripetibile)")
	f.BoolVar(&flagNewArtists, "new-artists", false, "solo artisti assenti dalla tua libreria")
	f.StringVar(&flagPlaylist, "playlist", "", "crea una playlist privata con questo nome (con -i: playlist usata dal tasto a)")
	f.BoolVarP(&flagInteractive, "interactive", "i", false, "sfoglia i suggerimenti in modalità interattiva")
	f.BoolVar(&flagNoSync, "no-sync", false, "non aggiornare automaticamente la cache della libreria")
	f.BoolVar(&flagSave, "save", false, "salva automaticamente i brani suggeriti nella tua libreria")
	discoverCmd.MarkFlagsMutuallyExclusive("save", "interactive")
}

func suggestionURIs(res []store.Suggestion) []string {
	uris := make([]string, len(res))
	for i, r := range res {
		uris[i] = r.URI
	}
	return uris
}

// ensureSynced aggiorna la cache della libreria se più vecchia di 24 ore (salvo --no-sync).
func ensureSynced(ctx context.Context, e *env) error {
	if flagNoSync {
		return nil
	}
	last, err := e.st.LastSync()
	if err != nil {
		return err
	}
	if time.Since(last) <= 24*time.Hour {
		return nil
	}
	logf("La cache della libreria è vecchia o assente: sincronizzo…")
	return profile.Sync(ctx, e.sp, e.st, logf)
}

func createPlaylist(ctx context.Context, sp *spotify.Client, name, description string, res []store.Suggestion) (*spotify.Playlist, error) {
	pl, err := sp.CreatePlaylist(ctx, name, description+" il "+time.Now().Format("02/01/2006"))
	if err != nil {
		return nil, fmt.Errorf("creazione playlist: %w", err)
	}
	if err := sp.AddToPlaylist(ctx, pl.ID, suggestionURIs(res)); err != nil {
		return nil, fmt.Errorf("aggiunta brani alla playlist: %w", err)
	}
	return pl, nil
}

// printTable stampa i suggerimenti come elenco di schede, adattato alla
// larghezza del terminale (una tabella a 6 colonne non ci sta quasi mai).
// notes, se presente, aggiunge un'informazione alla riga artista/album di ogni brano.
func printTable(res []store.Suggestion, notes []string) {
	width := 100
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		width = min(w, 100)
	}
	const indent = 5
	textW := max(width-indent, 30)

	num := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(indent - 1).Align(lipgloss.Right).MarginRight(1)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	artist := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	wrap := lipgloss.NewStyle().Width(textW)
	reason := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Italic(true)
	link := dim.Underline(true)
	pad := strings.Repeat(" ", indent)

	var b strings.Builder
	for i, r := range res {
		album := r.Album
		if r.Year != "" {
			album += " (" + r.Year + ")"
		}
		b.WriteString("\n")
		b.WriteString(num.Render(fmt.Sprintf("%d.", i+1)) + title.Render(r.Title) + "\n")
		meta := " · " + album
		if i < len(notes) && notes[i] != "" {
			meta += " · " + notes[i]
		}
		b.WriteString(pad + artist.Render(r.Artist) + dim.Render(meta) + "\n")
		if r.Reason != "" {
			for _, line := range strings.Split(wrap.Render(r.Reason), "\n") {
				b.WriteString(pad + reason.Render(strings.TrimRight(line, " ")) + "\n")
			}
		}
		if r.URL != "" {
			b.WriteString(pad + link.Render(r.URL) + "\n")
		}
	}
	fmt.Print(b.String())
}

// actions implementa tui.Actions.
type actions struct {
	sp           *spotify.Client
	st           *store.Store
	playlistName string

	mu       sync.Mutex
	playlist *spotify.Playlist
}

func (a *actions) Like(ctx context.Context, s store.Suggestion) error {
	if err := a.sp.SaveToLibrary(ctx, []string{s.URI}); err != nil {
		return err
	}
	return a.st.SetFeedback(s.Key, "like")
}

func (a *actions) Skip(_ context.Context, s store.Suggestion) error {
	return a.st.SetFeedback(s.Key, "skip")
}

func (a *actions) AddToPlaylist(ctx context.Context, s store.Suggestion) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.playlist == nil {
		pl, err := a.sp.CreatePlaylist(ctx, a.playlistName, "Brani scelti con tunescout")
		if err != nil {
			return "", err
		}
		a.playlist = pl
	}
	return a.playlist.Name, a.sp.AddToPlaylist(ctx, a.playlist.ID, []string{s.URI})
}

func (a *actions) Open(s store.Suggestion) {
	if s.URL != "" {
		spotify.OpenURL(s.URL)
	} else {
		fmt.Fprintln(os.Stderr, s.URI)
	}
}
