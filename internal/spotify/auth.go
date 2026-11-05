package spotify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/rubenpari/tunescout/internal/config"
)

var Scopes = []string{
	"user-library-read",
	"user-library-modify",
	"user-top-read",
	"playlist-read-private",
	"playlist-modify-private",
	"playlist-modify-public", // richiesto da PUT /me/library
	"user-follow-modify",     // richiesto da PUT /me/library
}

func oauthConfig(clientID string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    clientID,
		RedirectURL: config.RedirectURL,
		Scopes:      Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://accounts.spotify.com/authorize",
			TokenURL:  "https://accounts.spotify.com/api/token",
			AuthStyle: oauth2.AuthStyleInParams, // PKCE: nessun client secret
		},
	}
}

// Login esegue il flusso OAuth Authorization Code + PKCE con callback locale.
func Login(ctx context.Context, clientID string) error {
	conf := oauthConfig(clientID)
	verifier := oauth2.GenerateVerifier()
	state := randomState()

	u, _ := url.Parse(config.RedirectURL)
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return fmt.Errorf("impossibile ascoltare su %s: %w", u.Host, err)
	}

	type result struct {
		tok *oauth2.Token
		err error
	}
	done := make(chan result, 1)
	var once sync.Once
	finish := func(r result) { once.Do(func() { done <- r }) }

	mux := http.NewServeMux()
	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "Autorizzazione negata: "+e, http.StatusForbidden)
			finish(result{err: fmt.Errorf("autorizzazione negata: %s", e)})
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state non valido", http.StatusBadRequest)
			finish(result{err: errors.New("state OAuth non corrispondente")})
			return
		}
		tok, err := conf.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(verifier))
		if err != nil {
			http.Error(w, "Scambio token fallito", http.StatusInternalServerError)
			finish(result{err: fmt.Errorf("scambio token: %w", err)})
			return
		}
		fmt.Fprintln(w, "Login completato! Puoi chiudere questa finestra e tornare al terminale.")
		finish(result{tok: tok})
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	fmt.Println("Apri questo URL nel browser per autorizzare tunescout:")
	fmt.Println(authURL)
	openBrowser(authURL)

	select {
	case r := <-done:
		if r.err != nil {
			return r.err
		}
		return saveToken(r.tok)
	case <-time.After(5 * time.Minute):
		return errors.New("timeout: nessuna risposta dal browser entro 5 minuti")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// savingTokenSource persiste il token ogni volta che viene rinnovato.
type savingTokenSource struct {
	mu   sync.Mutex
	base oauth2.TokenSource
	last string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		_ = saveToken(t)
	}
	return t, nil
}

// HTTPClient restituisce un client autenticato con refresh automatico del token.
func HTTPClient(ctx context.Context, clientID string) (*http.Client, error) {
	tok, err := loadToken()
	if err != nil {
		return nil, err
	}
	ts := &savingTokenSource{
		base: oauthConfig(clientID).TokenSource(ctx, tok),
		last: tok.AccessToken,
	}
	return oauth2.NewClient(ctx, oauth2.ReuseTokenSource(nil, ts)), nil
}

var ErrNotLoggedIn = errors.New("non sei autenticato: esegui prima `tunescout login`")

func loadToken() (*oauth2.Token, error) {
	p, err := config.TokenPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("token.json non valido (riesegui login): %w", err)
	}
	return &t, nil
}

func saveToken(t *oauth2.Token) error {
	p, err := config.TokenPath()
	if err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// OpenURL apre un URL nel browser predefinito (best effort, anche su WSL).
func OpenURL(u string) { openBrowser(u) }

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", u)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case isWSL():
		cmd = exec.Command("cmd.exe", "/c", "start", "", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}

func isWSL() bool {
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(b)), "microsoft")
}
