package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

const serverUsage = `server <subcommand>
  list                          List dedicated servers
  describe <hostname>           Show all server fields
  start|stop|restart <hostname> Power actions
  reinstall <hostname> --template <id|name> [--raid <type>] [--startup-script <id>] [--yes]
                                Reinstall OS. WIPES ALL DATA. Asks for confirmation unless --yes
  kvm <hostname> [--client-ip <ip>] [--whitelabel]
                                Order KVM console access. Access details arrive by email/panel`

func handleServer(ctx context.Context, c *client.Client, a args) error {
	switch a.pos(1) {
	case "list":
		servers, err := c.ListDedicateds(ctx)
		if err != nil {
			return err
		}
		return printTable(a, servers, "hostname", "label", "status", "network_interface_status", "maintenance", "expire_at")

	case "describe":
		rest, err := a.need(1, "server describe <hostname>")
		if err != nil {
			return err
		}
		s, err := c.GetDedicated(ctx, rest[0])
		if err != nil {
			return err
		}
		return printObject(a, s)

	case "start", "stop", "restart":
		action := a.pos(1)
		rest, err := a.need(1, "server "+action+" <hostname>")
		if err != nil {
			return err
		}
		if err := c.PowerDedicated(ctx, rest[0], action); err != nil {
			return err
		}
		return done("%s task created for %s", action, rest[0])

	case "reinstall":
		usage := "server reinstall <hostname> --template <id|name> [--raid <type>] [--startup-script <id>] [--yes]"
		rest, err := a.need(1, usage)
		if err != nil {
			return err
		}
		if a.str("template") == "" {
			return usageError(usage)
		}
		hostname := rest[0]
		tmpl, err := resolveTemplate(ctx, c, a.str("template"))
		if err != nil {
			return err
		}
		opts := client.ReinstallOptions{TemplateID: tmpl.Int64("id"), RaidValue: a.str("raid")}
		if s := a.str("startup-script"); s != "" {
			if opts.StartupScriptID, err = strconv.ParseInt(s, 10, 64); err != nil {
				return fmt.Errorf("--startup-script must be numeric id: %w", err)
			}
		}
		if !a.bool("yes") {
			if err := confirm(fmt.Sprintf("Reinstall %s with %q? ALL DATA WILL BE LOST.", hostname, tmpl.String("name")), hostname); err != nil {
				return err
			}
		}
		if err := c.ReinstallDedicated(ctx, hostname, opts); err != nil {
			return err
		}
		return done("Reinstall task created for %s (%s)", hostname, tmpl.String("name"))

	case "kvm":
		rest, err := a.need(1, "server kvm <hostname> [--client-ip <ip>] [--whitelabel]")
		if err != nil {
			return err
		}
		if err := c.OrderKVM(ctx, rest[0], a.str("client-ip"), a.bool("whitelabel")); err != nil {
			return err
		}
		return done("KVM ordered for %s. Access details will be sent by MEVSpace (email / panel alerts).", rest[0])

	default:
		return usageError(serverUsage)
	}
}

// resolveTemplate finds a dedicated template by numeric id or case-insensitive name.
func resolveTemplate(ctx context.Context, c *client.Client, ref string) (client.Object, error) {
	templates, err := c.ListTemplates(ctx, "dedicated")
	if err != nil {
		return nil, err
	}
	for _, t := range templates {
		if t.String("id") == ref || strings.EqualFold(t.String("name"), ref) {
			return t, nil
		}
	}
	return nil, fmt.Errorf("template %q not found, see 'mevspace template list'", ref)
}

// confirm requires the user to type expected (e.g. the hostname) to proceed.
func confirm(prompt, expected string) error {
	fmt.Fprintf(os.Stderr, "%s\nType %q to confirm: ", prompt, expected)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != expected {
		return fmt.Errorf("aborted")
	}
	return nil
}
