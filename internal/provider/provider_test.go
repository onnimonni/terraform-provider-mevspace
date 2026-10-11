package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestSchemasValid(t *testing.T) {
	ctx := context.Background()
	p := New("test")()
	for _, f := range p.Resources(ctx) {
		var resp resource.SchemaResponse
		f().Schema(ctx, resource.SchemaRequest{}, &resp)
		resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
	}
	for _, f := range p.DataSources(ctx) {
		var resp datasource.SchemaResponse
		f().Schema(ctx, datasource.SchemaRequest{}, &resp)
		resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
	}
}
