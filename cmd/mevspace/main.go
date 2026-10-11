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

Auth: MEVSPACE_API_TOKEN env var, else the active context. Context tokens are kept in the
OS keyring (macOS Keychain); context names live in ~/.config/mevspace/cli.toml.`

// commandUsage lets --help print usage without touching the API or credentials.
var commandUsage = map[string]string{
	"server":         serverUsage,
	"ssh-key":        sshKeyUsage,
	"template":       templateUsage,
	"startup-script": startupScriptUsage,
	"context":        contextUsage,
}

func main() {
	a, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}
	if a.bool("help") {
		if u, ok := commandUsage[a.pos(0)]; ok {
			fmt.Println(usageError(u).Error())
		} else {
			fmt.Println(helpText)
		}
		return
	}
	if err := run(context.Background(), a); err != nil {
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

	// A missing token is reported only if the API rejects the request, so usage errors
	// and public endpoints (templates) work without one.
	token, err := resolveToken(a.str("context"))
	if err != nil && !errors.Is(err, errNoToken) {
		return err
	}
	c := client.New(os.Getenv("MEVSPACE_ENDPOINT"), token)
	c.UserAgent = "mevspace-cli/" + version
	return enhanceAuthError(h(ctx, c, a), token == "")
}

var errNoToken = errors.New(`no API token: set MEVSPACE_API_TOKEN or run 'mevspace context create <name>'

Get a token from https://panel.mevspace.com (ask MEVSpace support if API access is not visible).`)

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
	explicit := contextName != ""
	if !explicit {
		contextName = cfg.ActiveContext
	}
	if contextName == "" {
		return "", errNoToken
	}
	c := cfg.get(contextName)
	if c == nil {
		if explicit {
			return "", fmt.Errorf("context %q not found", contextName)
		}
		return "", fmt.Errorf("active context %q not found in config; run 'mevspace context use <name>'", contextName)
	}
	token, err := loadToken(c)
	if err != nil || token == "" {
		return "", fmt.Errorf("no token stored for context %q in %s (recreate it with 'mevspace context delete' + 'create'): %v",
			contextName, tokenBackend(c), err)
	}
	return token, nil
}

func enhanceAuthError(err error, noToken bool) error {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		return err
	}
	if noToken {
		return errNoToken
	}
	return fmt.Errorf("%w\n\nAuthentication failed: check MEVSPACE_API_TOKEN or the active context ('mevspace context active')", err)
}

// args holds positional arguments and --flags.
type args struct {
	positional []string
	flags      map[string]string
}

var (
	boolFlags  = map[string]bool{"help": true, "json": true, "yes": true, "whitelabel": true, "insecure-plaintext": true}
	valueFlags = map[string]bool{"context": true, "template": true, "raid": true, "startup-script": true, "client-ip": true, "type": true}
)

// parseArgs rejects unknown flags and missing values, and never treats another flag as a value,
// so e.g. `--raid --help` cannot swallow --help. Everything after `--` is positional.
func parseArgs(in []string) (args, error) {
	a := args{flags: map[string]string{}}
	for i := 0; i < len(in); i++ {
		s := in[i]
		switch {
		case s == "--":
			a.positional = append(a.positional, in[i+1:]...)
			return a, nil
		case s == "-h":
			a.flags["help"] = "true"
			continue
		case s == "-" || !strings.HasPrefix(s, "-"):
			a.positional = append(a.positional, s)
			continue
		case !strings.HasPrefix(s, "--"):
			return a, fmt.Errorf("unknown flag %q", s)
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(s, "--"), "=")
		switch {
		case boolFlags[name]:
			if hasVal {
				return a, fmt.Errorf("flag --%s takes no value", name)
			}
			val = "true"
		case valueFlags[name]:
			if !hasVal {
				if i+1 >= len(in) || strings.HasPrefix(in[i+1], "-") {
					return a, fmt.Errorf("flag --%s needs a value", name)
				}
				i++
				val = in[i]
			}
			if val == "" {
				return a, fmt.Errorf("flag --%s needs a value", name)
			}
		default:
			return a, fmt.Errorf("unknown flag --%s", name)
		}
		a.flags[name] = val
	}
	return a, nil
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
	if len(rest) < n {
		return nil, usageError(usage)
	}
	return rest, nil
}

type usageError string

func (u usageError) Error() string { return "usage: mevspace " + string(u) }
