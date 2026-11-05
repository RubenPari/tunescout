package store

import (
	"path/filepath"
	"testing"
)

func TestRemovedAreSeen(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if seen, _ := st.Seen("ride|vapour trail", "abc"); seen {
		t.Fatal("brano non ancora registrato risulta visto")
	}
	if err := st.AddRemoved([]KnownTrack{{Key: "ride|vapour trail", SpotifyID: "abc", Artist: "Ride", Title: "Vapour Trail"}}); err != nil {
		t.Fatal(err)
	}
	// Una seconda rimozione dello stesso brano non deve fallire.
	if err := st.AddRemoved([]KnownTrack{{Key: "ride|vapour trail", SpotifyID: "abc", Artist: "Ride", Title: "Vapour Trail"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ key, id string }{{"ride|vapour trail", ""}, {"altra|chiave", "abc"}} {
		seen, err := st.Seen(c.key, c.id)
		if err != nil {
			t.Fatal(err)
		}
		if !seen {
			t.Errorf("Seen(%q, %q) = false, atteso true", c.key, c.id)
		}
	}
	// ReplaceLibrary (sync) non deve cancellare i brani rimossi.
	if err := st.ReplaceLibrary(nil, nil); err != nil {
		t.Fatal(err)
	}
	if seen, _ := st.Seen("ride|vapour trail", ""); !seen {
		t.Error("i brani rimossi devono sopravvivere a una sync")
	}
}
