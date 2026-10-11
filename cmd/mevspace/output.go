package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/aquasecurity/table"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTable prints objects as rows with the given columns, or JSON with --json.
func printTable(a args, objs []client.Object, cols ...string) error {
	if a.bool("json") {
		return printJSON(objs)
	}
	t := table.New(os.Stdout)
	t.SetHeaders(cols...)
	for _, o := range objs {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = fieldString(o, c)
		}
		t.AddRow(row...)
	}
	t.Render()
	return nil
}

// printObject prints all fields of one object as key/value rows, or JSON with --json.
func printObject(a args, o client.Object) error {
	if a.bool("json") {
		return printJSON(o)
	}
	keys := make([]string, 0, len(o))
	for k := range o {
		if k != "_links" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	t := table.New(os.Stdout)
	t.SetHeaders("Field", "Value")
	for _, k := range keys {
		t.AddRow(k, fieldString(o, k))
	}
	t.Render()
	return nil
}

// fieldString formats a field; *_at fields are unix timestamps.
func fieldString(o client.Object, key string) string {
	s := o.String(key)
	if len(key) > 3 && key[len(key)-3:] == "_at" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return time.Unix(n, 0).UTC().Format("2006-01-02 15:04 UTC")
		}
	}
	return s
}

func done(format string, v ...any) error {
	fmt.Printf(format+"\n", v...)
	return nil
}
