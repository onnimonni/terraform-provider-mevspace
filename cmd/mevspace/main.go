// Command mevspace is a CLI for the MEVSpace API, sharing the provider's client.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

var (
	version = "dev"
	commit  = "unknown"
)

const helpText = `mevspace - CLI for MEVSpace dedicated servers

Usage:
  mevspace <command> <subcommand> [args] [flags]

Commands:
  server          list | describe | start | stop | restart | reinstall | kvm
  ssh-key         list | describe | create | rename | delete
  template        list
  startup-script  list
  context         list | create | use | active | delete
  version

Global flags:
  --context <name>   Use this context instead of the active one
  --json             Print raw JSON
  -h, --help         Help for any command

Auth: MEVSPACE_API_TOKEN env var, else the active context (token in macOS Keychain,
or ~/.config/mevspace/cli.toml on other platforms).`

func main() {
	a := parseArgs(os.Args[1:])
	err := run(context.Background(), a)
	var usage usageError
	if errors.As(err, &usage) && a.bool("help") {
		fmt.Println(usage.Error())
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, a args) error {
	cmd := a.pos(0)
	switch cmd {
	case "", "help":
		fmt.Println(helpText)
		return nil
	case "version":
		fmt.Printf("mevspace version %s (commit: %s)\n", version, commit)
		return nil
	case "context":
		return handleContext(a)
	}

	handlers := map[string]func(context.Context, *client.Client, args) error{
		"server":         handleServer,
		"ssh-key":        handleSSHKey,
		"template":       handleTemplate,
		"startup-script": handleStartupScript,
	}
	h, ok := handlers[cmd]
	if !ok {
		return fmt.Errorf("unknown command %q, see 'mevspace help'", cmd)
	}
	// Help needs no credentials.
	if a.bool("help") {
		return h(ctx, nil, a)
	}

	// Templates are public; other commands require a token.
	token, err := resolveToken(a.str("context"))
	if err != nil && cmd != "template" {
		return err
	}
	c := client.New(os.Getenv("MEVSPACE_ENDPOINT"), token)
	c.UserAgent = "mevspace-cli/" + version
	return enhanceAuthError(h(ctx, c, a))
}

func resolveToken(contextName string) (string, error) {
	if contextName == "" {
		if t := os.Getenv("MEVSPACE_API_TOKEN"); t != "" {
			return t, nil
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}
	if contextName == "" {
		contextName = cfg.ActiveContext
	}
	if c := cfg.get(contextName); c != nil {
		token, err := loadToken(c)
		if err != nil {
			return "", fmt.Errorf("reading token for context %q from %s: %w", contextName, secretBackend, err)
		}
		if token != "" {
			return token, nil
		}
	}
	if contextName != "" {
		return "", fmt.Errorf("context %q not found", contextName)
	}
	return "", errors.New(`no API token: set MEVSPACE_API_TOKEN or run 'mevspace context create <name>'

Get a token from https://panel.mevspace.com (ask MEVSpace support if API access is not visible).`)
}

func enhanceAuthError(err error) error {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
		return fmt.Errorf("%w\n\nAuthentication failed: check MEVSPACE_API_TOKEN or the active context ('mevspace context active')", err)
	}
	return err
}

// args holds positional arguments and --flags. Flags in boolFlags take no value.
type args struct {
	positional []string
	flags      map[string]string
}

var boolFlags = map[string]bool{"help": true, "json": true, "yes": true, "whitelabel": true, "insecure-plaintext": true}

func parseArgs(in []string) args {
	a := args{flags: map[string]string{}}
	for i := 0; i < len(in); i++ {
		s := in[i]
		if s == "-h" {
			s = "--help"
		}
		if !strings.HasPrefix(s, "--") || s == "--" {
			a.positional = append(a.positional, s)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(s, "--"), "=")
		if !hasVal {
			if boolFlags[name] {
				val = "true"
			} else if i+1 < len(in) {
				i++
				val = in[i]
			}
		}
		a.flags[name] = val
	}
	return a
}

func (a args) pos(i int) string {
	if i < len(a.positional) {
		return a.positional[i]
	}
	return ""
}

func (a args) str(name string) string { return a.flags[name] }
func (a args) bool(name string) bool  { return a.flags[name] == "true" }

// need returns positional args from index 2 on, or a usage error when fewer than n.
func (a args) need(n int, usage string) ([]string, error) {
	rest := a.positional[min(2, len(a.positional)):]
	if len(rest) < n || a.bool("help") {
		return nil, usageError(usage)
	}
	return rest, nil
}

type usageError string

func (u usageError) Error() string { return "usage: mevspace " + string(u) }
