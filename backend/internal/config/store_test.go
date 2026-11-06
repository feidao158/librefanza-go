package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsInstalledConfiguration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	store := NewStore(dir)
	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("empty store: found=%v err=%v", found, err)
	}
	want := Config{Database: Database{Driver: "postgres", Host: "localhost", Port: 5432, Name: "librefanza", User: "app", Password: "secret"}, Installed: true}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load()
	if err != nil || !found || got != want {
		t.Fatalf("loaded config: got=%+v found=%v err=%v", got, found, err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions: info=%v err=%v", info, err)
	}
}

func TestStoreRejectsCorruptConfiguration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	_, found, err := NewStore(dir).Load()
	if !found || err == nil {
		t.Fatalf("corrupt config must fail closed: found=%v err=%v", found, err)
	}
}
