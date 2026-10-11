package main

import (
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestParseArgs(t *testing.T) {
	a := parseArgs([]string{"server", "reinstall", "--template", "Debian 13", "host1", "--yes", "--raid=1", "-h"})
	if a.pos(0) != "server" || a.pos(1) != "reinstall" || a.pos(2) != "host1" {
		t.Fatalf("positional: %v", a.positional)
	}
	if a.str("template") != "Debian 13" || a.str("raid") != "1" || !a.bool("yes") || !a.bool("help") {
		t.Fatalf("flags: %v", a.flags)
	}
	if _, err := a.need(1, "x"); err == nil {
		t.Fatal("need should return usage when --help given")
	}
}

func TestContextRoundTrip(t *testing.T) {
	t.Setenv("MEVSPACE_CONFIG", t.TempDir()+"/cli.toml")
	t.Setenv("MEVSPACE_API_TOKEN", "")

	if err := saveConfig(&Config{ActiveContext: "a", Contexts: []Context{{Name: "a", Token: "t1"}, {Name: "b", Token: "t2"}}}); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolveToken(""); err != nil || tok != "t1" {
		t.Fatalf("active: %q %v", tok, err)
	}
	if tok, err := resolveToken("b"); err != nil || tok != "t2" {
		t.Fatalf("named: %q %v", tok, err)
	}
	if _, err := resolveToken("missing"); err == nil {
		t.Fatal("expected error for missing context")
	}

	t.Setenv("MEVSPACE_API_TOKEN", "env")
	if tok, _ := resolveToken(""); tok != "env" {
		t.Fatalf("env should win, got %q", tok)
	}
}

func TestContextTokenInKeyring(t *testing.T) {
	keyring.MockInit()
	t.Setenv("MEVSPACE_CONFIG", t.TempDir()+"/cli.toml")
	t.Setenv("MEVSPACE_API_TOKEN", "")

	c := Context{Name: "k"}
	if _, err := storeToken(&c, "secret", false); err != nil || c.Token != "" {
		t.Fatalf("token must go to keyring, not config: %v %q", err, c.Token)
	}
	if err := saveConfig(&Config{ActiveContext: "k", Contexts: []Context{c}}); err != nil {
		t.Fatal(err)
	}
	p, _ := configPath()
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "secret") {
		t.Fatalf("config file contains token:\n%s", b)
	}
	if tok, err := resolveToken(""); err != nil || tok != "secret" {
		t.Fatalf("resolve: %q %v", tok, err)
	}
	if err := deleteToken(&c); err != nil {
		t.Fatal(err)
	}
	if _, err := loadToken(&c); err == nil {
		t.Fatal("token should be gone")
	}
}
