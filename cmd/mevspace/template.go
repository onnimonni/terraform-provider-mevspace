package main

import (
	"context"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

func handleTemplate(ctx context.Context, c *client.Client, a args) error {
	if a.pos(1) != "list" {
		return usageError("template list [--type dedicated|vps|vps_cloud]   (default: dedicated)")
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
		return usageError("startup-script list")
	}
	scripts, err := c.ListStartupScripts(ctx)
	if err != nil {
		return err
	}
	return printTable(a, scripts, "id", "name", "label")
}
