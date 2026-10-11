package main

import (
	"context"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

const (
	templateUsage      = "template list [--type dedicated|vps|vps_cloud]   (default: dedicated)"
	startupScriptUsage = "startup-script list"
)

func handleTemplate(ctx context.Context, c *client.Client, a args) error {
	if a.pos(1) != "list" {
		return usageError(templateUsage)
	}
	typ := a.str("type")
	if typ == "" {
		typ = "dedicated"
	}
	templates, err := c.ListTemplates(ctx, typ)
	if err != nil {
		return err
	}
	return printTable(a, templates, "id", "name", "type")
}

func handleStartupScript(ctx context.Context, c *client.Client, a args) error {
	if a.pos(1) != "list" {
		return usageError(startupScriptUsage)
	}
	scripts, err := c.ListStartupScripts(ctx)
	if err != nil {
		return err
	}
	return printTable(a, scripts, "id", "name", "label")
}
