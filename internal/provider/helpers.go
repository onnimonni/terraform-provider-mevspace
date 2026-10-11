package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

func clientFromProviderData(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", data))
		return nil
	}
	return c
}

// attributesMap exposes all API fields as map(string), since response schemas are undocumented.
func attributesMap(ctx context.Context, obj client.Object, diags *diag.Diagnostics) types.Map {
	m, d := types.MapValueFrom(ctx, types.StringType, obj.Flatten())
	diags.Append(d...)
	return m
}

func optionalString(o client.Object, key string) types.String {
	if v, ok := o[key]; ok && v != nil {
		return types.StringValue(client.Stringify(v))
	}
	return types.StringNull()
}
