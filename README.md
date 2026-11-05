# tunescout

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

CLI per **scoprire musica nuova** partendo dalla tua libreria Spotify, e per **fare pulizia**
dei brani salvati che non ascolti più. I suggerimenti sono generati da **Claude** e verificati
uno per uno su Spotify.

---

## Indice

- [Come funziona](#come-funziona)
- [Requisiti](#requisiti)
- [Installazione](#installazione)
- [Configurazione](#configurazione)
- [Guida rapida](#guida-rapida)
- [Comandi](#comandi)
- [Modalità interattiva](#modalità-interattiva)
- [Opzioni del modello](#opzioni-del-modello)
- [Dati locali e privacy](#dati-locali-e-privacy)
- [Risoluzione problemi](#risoluzione-problemi)
- [Struttura del progetto](#struttura-del-progetto)
- [Sviluppo](#sviluppo)
- [Licenza](#licenza)

## Come funziona

Spotify non offre più alle nuove app gli endpoint di raccomandazione
(`recommendations`, `related-artists`, `audio-features`), quindi tunescout:

1. **legge la tua libreria**: brani salvati, top artisti e top brani su 3 periodi
   (4 settimane, 6 mesi, 1 anno), playlist create da te;
2. **costruisce un profilo di gusto** e chiede a Claude dei brani adatti a te
   (tenendo conto di mood, generi e del tuo feedback precedente);
3. **verifica ogni suggerimento su Spotify**, scartando quelli inesistenti o inventati;
4. **esclude** i brani che hai già in libreria, quelli già suggeriti in passato e quelli
   che hai rimosso con `prune`.

## Requisiti

- [Go](https://go.dev/dl/) 1.26 o superiore (solo per compilare)
- Un account Spotify (Free o Premium)
- Una API key Anthropic ([platform.claude.com](https://platform.claude.com))

## Installazione

Con `go install`:

```bash
go install github.com/rubenpari/tunescout@latest
```

Oppure dai sorgenti:

```bash
git clone https://github.com/RubenPari/tunescout.git
cd tunescout
go build -o tunescout .
```

## Configurazione

### 1. Crea un'app Spotify

1. Vai su <https://developer.spotify.com/dashboard> e crea una nuova app.
2. **Redirect URI**: `http://127.0.0.1:8888/callback` (deve corrispondere esattamente).
3. **API**: seleziona *Web API*.
4. In **User Management** aggiungi il tuo account Spotify: le app in *Development Mode*
   funzionano solo per gli utenti elencati.
5. Copia il **Client ID** (il client secret non serve: il login usa OAuth PKCE).

### 2. Imposta la API key di Claude

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

Aggiungila al tuo `~/.bashrc` / `~/.zshrc` per non doverla reimpostare.

### 3. Variabili d'ambiente (opzionali)

| Variabile           | Descrizione                                              |
|---------------------|----------------------------------------------------------|
| `ANTHROPIC_API_KEY` | API key Anthropic (**obbligatoria**)                     |
| `SPOTIFY_CLIENT_ID` | Client ID Spotify, alternativa a `login --client-id`     |
| `TUNESCOUT_MODEL`   | Modello Claude da usare (default `claude-opus-5-5`)      |

### 4. File di configurazione

Dopo il login, tunescout salva le impostazioni in `~/.config/tunescout/config.json`:

```json
{
  "spotify_client_id": "<CLIENT_ID>",
  "model": "claude-opus-5-5",
  "effort": "medium"
}
```

`model` ed `effort` sono opzionali. Le variabili d'ambiente hanno la precedenza sul file,
e i flag `--model` / `--effort` hanno la precedenza su entrambi.

## Guida rapida

```bash
tunescout login --client-id <CLIENT_ID>   # si apre il browser per autorizzare l'app
tunescout sync                            # scarica la libreria nella cache locale
tunescout discover -i                     # sfoglia i suggerimenti in modalità interattiva
```

## Comandi

Tutti i comandi accettano i flag globali `--model` ed `--effort`
(vedi [Opzioni del modello](#opzioni-del-modello)). Usa `tunescout <comando> --help` per i dettagli.

### `login`

Autorizza tunescout ad accedere al tuo account Spotify tramite OAuth PKCE
(nessun client secret). Avvia un server locale su `127.0.0.1:8888` e attende la risposta
del browser per al massimo 5 minuti.

| Flag          | Descrizione                       |
|---------------|-----------------------------------|
| `--client-id` | Client ID dell'app Spotify        |

### `sync`

Scarica la libreria Spotify nella cache locale. `discover` e `prune` lo eseguono
automaticamente se l'ultima sincronizzazione ha più di 24 ore.

```bash
tunescout sync
```

### `discover`

Suggerisce brani nuovi in base ai tuoi gusti.

| Flag                  | Default | Descrizione                                                         |
|-----------------------|---------|---------------------------------------------------------------------|
| `-n`, `--count`       | `20`    | numero di brani da suggerire (1–50)                                 |
| `--mood`              |         | atmosfera desiderata, es. `"energico"` o `"per studiare"`           |
| `--genre`             |         | genere desiderato (ripetibile)                                      |
| `--new-artists`       | `false` | solo artisti assenti dalla tua libreria                             |
| `--playlist`          |         | crea una playlist privata con questo nome (con `-i`: playlist del tasto `a`) |
| `-i`, `--interactive` | `false` | sfoglia i suggerimenti in modalità interattiva                      |
| `--save`              | `false` | salva automaticamente i suggerimenti nella libreria (non combinabile con `-i`) |
| `--no-sync`           | `false` | non aggiornare automaticamente la cache della libreria              |

```bash
tunescout discover                                    # 20 suggerimenti in tabella
tunescout discover -n 30 --mood "notturno, malinconico"
tunescout discover --genre shoegaze --genre dream-pop --new-artists
tunescout discover --playlist "Scoperte"              # crea una playlist privata
tunescout discover --save                             # salva i suggerimenti nella libreria
tunescout discover -i                                 # modalità interattiva
```

### `prune`

Suggerisce brani salvati da eliminare perché poco ascoltati e lontani dai tuoi gusti.

| Flag            | Default | Descrizione                                                       |
|-----------------|---------|-------------------------------------------------------------------|
| `-n`, `--count` | `20`    | numero massimo di brani da proporre (1–50)                        |
| `--min-age`     | `90`    | considera solo i brani salvati da almeno questi giorni            |
| `--playlist`    |         | raccoglie i brani proposti in una playlist privata                |
| `--remove`      | `false` | dopo l'elenco chiede conferma e rimuove i brani dalla libreria    |
| `--no-sync`     | `false` | non aggiornare automaticamente la cache della libreria            |

```bash
tunescout prune                           # elenco dei candidati, nessuna modifica
tunescout prune -n 40 --min-age 365       # solo brani salvati da almeno un anno
tunescout prune --playlist "Da eliminare" # li raccoglie in una playlist per riascoltarli
tunescout prune --remove                  # li rimuove dalla libreria, dopo conferma
```

**Come vengono scelti i candidati.** Spotify non espone il numero di ascolti, quindi un brano
è considerato "poco ascoltato" se:

- è salvato da almeno `--min-age` giorni;
- non è tra i tuoi top brani delle ultime 4 settimane / 6 mesi / 1 anno;
- nessuno dei suoi artisti è tra i 50 che ascolti di più.

Tra questi, Claude sceglie quelli meno in linea con il tuo profilo.

**Sicurezza.** Senza `--remove`, `prune` non elimina nulla. Con `--remove` chiede conferma
(`s`/`N`) e richiede un terminale interattivo. Abbinato a `--playlist`, la playlist resta
come copia di sicurezza. I brani rimossi vengono registrati nel database locale e
`discover` non li riproporrà.

### `history`

Mostra i suggerimenti passati e il tuo feedback.

| Flag            | Default | Descrizione                         |
|-----------------|---------|-------------------------------------|
| `-n`, `--limit` | `50`    | numero massimo di voci (0 = tutte)  |
| `--feedback`    |         | filtra per feedback: `like` o `skip` |

```bash
tunescout history
tunescout history --feedback like
```

## Modalità interattiva

Con `tunescout discover -i`:

| Tasto   | Azione                                                              |
|---------|---------------------------------------------------------------------|
| `↑` `↓` | naviga tra i suggerimenti                                           |
| `l`     | like: salva il brano nella libreria                                 |
| `s`     | skip                                                                |
| `a`     | aggiungi alla playlist (`--playlist`, default "tunescout · scoperte") |
| `o`     | apri il brano in Spotify                                            |
| `q`     | esci                                                                |

Like e skip vengono usati come segnale per i suggerimenti successivi.

## Opzioni del modello

- `--model` / `$TUNESCOUT_MODEL` — default `claude-opus-5-5`
- `--effort low|medium|high|xhigh|max` — default `medium`. Un effort più alto dà
  suggerimenti più ragionati ma è più lento e costoso.

Sui modelli che lo supportano è attivo il fallback lato server in caso di rifiuto
(`fallbacks: "default"`).

## Dati locali e privacy

Tutto è salvato in `~/.config/tunescout/` (cartella con permessi `700`):

| File            | Contenuto                                                   |
|-----------------|-------------------------------------------------------------|
| `config.json`   | Client ID Spotify, modello, effort                          |
| `token.json`    | token OAuth Spotify (permessi `600`)                        |
| `tunescout.db`  | SQLite: cache libreria, profilo, storico suggerimenti e feedback |

I dati della libreria vengono inviati solo a Spotify e, sotto forma di profilo di gusto,
all'API di Anthropic per generare i suggerimenti. Per ripartire da zero basta cancellare
la cartella.

## Risoluzione problemi

| Problema | Soluzione |
|----------|-----------|
| `403` / errori di permesso da Spotify | Aggiungi il tuo account in *User Management* nella dashboard dell'app Spotify. |
| `INVALID_CLIENT: Invalid redirect URI` | Il Redirect URI dell'app deve essere esattamente `http://127.0.0.1:8888/callback`. |
| `impossibile ascoltare su 127.0.0.1:8888` | La porta 8888 è occupata: chiudi il processo che la usa e riprova. |
| `non sei autenticato` / `token.json non valido` | Riesegui `tunescout login`. |
| `client ID Spotify mancante` | `tunescout login --client-id <ID>` oppure imposta `SPOTIFY_CLIENT_ID`. |
| `Anthropic: credenziali non valide` | Imposta `ANTHROPIC_API_KEY`. |
| `Anthropic: rate limit raggiunto` | Attendi qualche minuto e riprova. |
| `risposta troncata (max_tokens)` | Riduci il numero di brani con `-n`. |
| Pochi suggerimenti restituiti | Alcuni suggerimenti non esistono su Spotify o li conosci già: aumenta `-n` o allenta i filtri. |

## Struttura del progetto

```
.
├── main.go
├── cmd/                 # comandi CLI (cobra): login, sync, discover, prune, history
└── internal/
    ├── config/          # percorsi e impostazioni
    ├── spotify/         # client Web API e login OAuth PKCE
    ├── store/           # database SQLite locale
    ├── profile/         # sincronizzazione libreria e profilo di gusto
    ├── llm/             # integrazione con Claude (suggerimenti e prune)
    ├── discover/        # pipeline: suggerimenti → verifica su Spotify → filtri
    ├── prune/           # selezione dei brani poco ascoltati
    ├── norm/            # normalizzazione di titoli e artisti per il confronto
    └── tui/             # interfaccia interattiva
```

## Sviluppo

```bash
go build -o tunescout .
go test ./...
go vet ./...
```

## Licenza

Distribuito con licenza [MIT](LICENSE).
