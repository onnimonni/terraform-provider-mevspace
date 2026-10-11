package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/term"
)

type Config struct {
	ActiveContext string    `toml:"active_context"`
	Contexts      []Context `toml:"contexts"`
}

type Context struct {
	Name  string `toml:"name"`
	Token string `toml:"token,omitempty"` // only used when the OS keyring is unavailable
}

func configPath() (string, error) {
	if p := os.Getenv("MEVSPACE_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mevspace", "cli.toml"), nil
}

func loadConfig() (*Config, error) {
	p, err := configPath()
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if _, err := toml.DecodeFile(p, cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}
	return cfg, nil
}

func saveConfig(cfg *Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func (c *Config) get(name string) *Context {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i]
		}
	}
	return nil
}

const contextUsage = `context <subcommand>
  list            List contexts
  create <name>   Create a context; prompts for the token (or reads it from stdin when piped).
                  Token goes to the OS keyring (macOS Keychain, service "mevspace-cli").
                  --insecure-plaintext stores it in the config file instead
  use <name>      Switch active context
  active          Show active context
  delete <name>   Delete a context`

func handleContext(a args) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	sub := a.pos(1)
	switch sub {
	case "list":
		if len(cfg.Contexts) == 0 {
			fmt.Println("No contexts. Create one with 'mevspace context create <name>'.")
		}
		for _, c := range cfg.Contexts {
			marker := " "
			if c.Name == cfg.ActiveContext {
				marker = "*"
			}
			fmt.Printf("%s %s\n", marker, c.Name)
		}
		return nil

	case "create":
		rest, err := a.need(1, "context create <name>")
		if err != nil {
			return err
		}
		name := rest[0]
		if cfg.get(name) != nil {
			return fmt.Errorf("context %q already exists", name)
		}
		token, err := readToken()
		if err != nil {
			return err
		}
		c := Context{Name: name}
		where, err := storeToken(&c, token, a.bool("insecure-plaintext"))
		if err != nil {
			return err
		}
		cfg.Contexts = append(cfg.Contexts, c)
		if len(cfg.Contexts) == 1 {
			cfg.ActiveContext = name
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("Context %q created (token stored in %s)\n", name, where)
		if cfg.ActiveContext == name {
			fmt.Printf("Context %q is now active\n", name)
		}
		return nil

	case "use":
		rest, err := a.need(1, "context use <name>")
		if err != nil {
			return err
		}
		if cfg.get(rest[0]) == nil {
			return fmt.Errorf("context %q not found", rest[0])
		}
		cfg.ActiveContext = rest[0]
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("Switched to context %q\n", rest[0])
		return nil

	case "active":
		if cfg.ActiveContext == "" {
			fmt.Println("No active context")
		} else {
			fmt.Println(cfg.ActiveContext)
		}
		fmt.Printf("Token storage: %s\n", secretBackend)
		if os.Getenv("MEVSPACE_API_TOKEN") != "" {
			fmt.Println("(MEVSPACE_API_TOKEN is set and takes precedence)")
		}
		return nil

	case "delete":
		rest, err := a.need(1, "context delete <name>")
		if err != nil {
			return err
		}
		if c := cfg.get(rest[0]); c != nil {
			if err := deleteToken(c); err != nil {
				return fmt.Errorf("deleting token from %s: %w", secretBackend, err)
			}
		}
		kept := cfg.Contexts[:0]
		for _, c := range cfg.Contexts {
			if c.Name != rest[0] {
				kept = append(kept, c)
			}
		}
		if len(kept) == len(cfg.Contexts) {
			return fmt.Errorf("context %q not found", rest[0])
		}
		cfg.Contexts = kept
		if cfg.ActiveContext == rest[0] {
			cfg.ActiveContext = ""
		}
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("Context %q deleted\n", rest[0])
		return nil

	default:
		return usageError(contextUsage)
	}
}

// readToken reads the token without echo from a terminal, or from piped stdin
// (e.g. `sops exec-env secrets.enc.env 'printenv MEVSPACE_API_TOKEN | mevspace context create x'`).
func readToken() (string, error) {
	var token string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "API token: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		token = string(b)
	} else {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading token from stdin: %w", err)
		}
		token = line
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token cannot be empty")
	}
	return token, nil
}
