package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

const sshKeyUsage = `ssh-key <subcommand>
  list                        List SSH keys
  describe <id|label>         Show SSH key
  create <label> <file|->     Upload public key from file or stdin
  rename <id|label> <label>   Change label
  delete <id|label>           Delete SSH key`

func handleSSHKey(ctx context.Context, c *client.Client, a args) error {
	switch a.pos(1) {
	case "list":
		keys, err := c.ListSSHKeys(ctx)
		if err != nil {
			return err
		}
		return printTable(a, keys, "id", "label", "value")

	case "describe":
		rest, err := a.need(1, "ssh-key describe <id|label>")
		if err != nil {
			return err
		}
		k, err := findSSHKey(ctx, c, rest[0])
		if err != nil {
			return err
		}
		return printObject(a, k)

	case "create":
		rest, err := a.need(2, "ssh-key create <label> <file|->")
		if err != nil {
			return err
		}
		value, err := readKeyFile(rest[1])
		if err != nil {
			return err
		}
		k, err := c.CreateSSHKey(ctx, rest[0], value)
		if err != nil {
			return err
		}
		return done("SSH key %q created (id %s)", rest[0], k.String("id"))

	case "rename":
		rest, err := a.need(2, "ssh-key rename <id|label> <label>")
		if err != nil {
			return err
		}
		k, err := findSSHKey(ctx, c, rest[0])
		if err != nil {
			return err
		}
		if err := c.UpdateSSHKey(ctx, k.String("id"), rest[1]); err != nil {
			return err
		}
		return done("SSH key %s renamed to %q", k.String("id"), rest[1])

	case "delete":
		rest, err := a.need(1, "ssh-key delete <id|label>")
		if err != nil {
			return err
		}
		k, err := findSSHKey(ctx, c, rest[0])
		if err != nil {
			return err
		}
		if err := c.DeleteSSHKey(ctx, k.String("id")); err != nil {
			return err
		}
		return done("SSH key %s (%s) deleted", k.String("id"), k.String("label"))

	default:
		return usageError(sshKeyUsage)
	}
}

// findSSHKey matches by id first, then by unique label.
func findSSHKey(ctx context.Context, c *client.Client, ref string) (client.Object, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return nil, err
	}
	var byLabel []client.Object
	for _, k := range keys {
		if k.String("id") == ref {
			return k, nil
		}
		if k.String("label") == ref {
			byLabel = append(byLabel, k)
		}
	}
	switch len(byLabel) {
	case 1:
		return byLabel[0], nil
	case 0:
		return nil, fmt.Errorf("ssh key %q not found", ref)
	default:
		return nil, fmt.Errorf("label %q matches %d keys, use the id", ref, len(byLabel))
	}
}

func readKeyFile(path string) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if strings.Contains(v, "PRIVATE KEY") {
		return "", fmt.Errorf("%s looks like a private key; pass the .pub file", path)
	}
	return v, nil
}
