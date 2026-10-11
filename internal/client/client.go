// Package client is a minimal MEVSpace API (https://api.mevspace.com/explore/) client.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultEndpoint = "https://api.mevspace.com"

// Object is a raw API object. Response schemas are undocumented, so keep them generic.
type Object map[string]any

type Client struct {
	endpoint  string
	token     string
	http      *http.Client
	UserAgent string
}

func New(endpoint, token string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		http: &http.Client{
			Timeout: 60 * time.Second,
			// The token travels in the query string, so never follow it to another host or to http.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
					return errors.New("refusing cross-host or scheme-changing redirect")
				}
				return nil
			},
		},
	}
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mevspace api: HTTP %d: %s", e.StatusCode, e.Message)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// do performs a request. Every returned error is scrubbed of the access token.
func (c *Client) do(ctx context.Context, method, path string, query, form url.Values) (*http.Response, []byte, error) {
	resp, data, err := c.doRaw(ctx, method, path, query, form)
	return resp, data, c.redact(err)
}

// redact removes the token (raw and URL-encoded) from an error, keeping APIError typed.
func (c *Client) redact(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	scrub := func(s string) string {
		for _, t := range []string{c.token, url.QueryEscape(c.token), url.PathEscape(c.token)} {
			s = strings.ReplaceAll(s, t, "[REDACTED]")
		}
		return s
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return &APIError{StatusCode: apiErr.StatusCode, Message: scrub(apiErr.Message)}
	}
	return errors.New(scrub(err.Error()))
}

// checkEndpoint requires https, except for loopback hosts (local testing).
func (c *Client) checkEndpoint() error {
	u, err := url.Parse(c.endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid endpoint %q", c.endpoint)
	}
	if u.Scheme == "https" {
		return nil
	}
	if host := u.Hostname(); u.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return fmt.Errorf("endpoint %q must use https", c.endpoint)
}

func (c *Client) doRaw(ctx context.Context, method, path string, query, form url.Values) (*http.Response, []byte, error) {
	if err := c.checkEndpoint(); err != nil {
		return nil, nil, err
	}
	if query == nil {
		query = url.Values{}
	}
	if c.token != "" {
		query.Set("access-token", c.token)
	}
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error embeds the full URL; never leak the access token.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			urlErr.URL = c.endpoint + path
		}
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, data, &APIError{StatusCode: resp.StatusCode, Message: errorMessage(data)}
	}
	return resp, data, nil
}

// errorMessage extracts a message from Yii2-style error bodies:
// {"name":"..","message":".."} or validation errors [{"field":"..","message":".."}].
func errorMessage(data []byte) string {
	var single struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &single) == nil && single.Message != "" {
		return single.Message
	}
	var list []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &list) == nil && len(list) > 0 {
		msgs := make([]string, 0, len(list))
		for _, e := range list {
			msgs = append(msgs, strings.TrimPrefix(e.Field+": "+e.Message, ": "))
		}
		return strings.Join(msgs, "; ")
	}
	return strings.TrimSpace(string(data))
}

func decode(data []byte, v any) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	return dec.Decode(v)
}

// list fetches every page of a Yii2 paginated collection.
func (c *Client) list(ctx context.Context, path string, query url.Values) ([]Object, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("per-page", "50")
	var all []Object
	for page := 1; ; page++ {
		query.Set("page", strconv.Itoa(page))
		resp, data, err := c.do(ctx, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, err
		}
		var items []Object
		if err := decode(data, &items); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", path, err)
		}
		all = append(all, items...)
		pages, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Page-Count"))
		if page >= pages || len(items) == 0 {
			return all, nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string) (Object, error) {
	_, data, err := c.do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var obj Object
	if err := decode(data, &obj); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return obj, nil
}

// --- Templates ---

// ListTemplates lists OS templates. templateType is one of dedicated, vps, vps_cloud or empty for all.
func (c *Client) ListTemplates(ctx context.Context, templateType string) ([]Object, error) {
	q := url.Values{}
	if templateType != "" {
		q.Set("type", templateType)
	}
	return c.list(ctx, "/templates", q)
}

// --- Startup scripts ---

func (c *Client) ListStartupScripts(ctx context.Context) ([]Object, error) {
	return c.list(ctx, "/startup-scripts", nil)
}

// --- SSH keys ---

func (c *Client) ListSSHKeys(ctx context.Context) ([]Object, error) {
	return c.list(ctx, "/client-ssh-keys", nil)
}

func (c *Client) GetSSHKey(ctx context.Context, id string) (Object, error) {
	return c.get(ctx, "/client-ssh-keys/"+url.PathEscape(id))
}

// CreateSSHKey creates a key. The API may or may not return the created object,
// so fall back to finding it by label and value.
func (c *Client) CreateSSHKey(ctx context.Context, label, value string) (Object, error) {
	form := url.Values{"label": {label}, "value": {value}}
	_, data, err := c.do(ctx, http.MethodPost, "/client-ssh-keys", nil, form)
	if err != nil {
		return nil, err
	}
	var obj Object
	if decode(data, &obj) == nil && obj.String("id") != "" {
		return obj, nil
	}
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return nil, err
	}
	// Newest first: prefer same label + same key material, else newest with the same label
	// (the API may normalise the value, e.g. drop the comment).
	var sameLabel Object
	for i := len(keys) - 1; i >= 0; i-- {
		k := keys[i]
		if k.String("label") != label {
			continue
		}
		if KeyMaterial(k.String("value")) == KeyMaterial(value) {
			return k, nil
		}
		if sameLabel == nil {
			sameLabel = k
		}
	}
	if sameLabel != nil {
		return sameLabel, nil
	}
	return nil, fmt.Errorf("ssh key %q created but not found in key list", label)
}

func (c *Client) UpdateSSHKey(ctx context.Context, id, label string) error {
	_, _, err := c.do(ctx, http.MethodPatch, "/client-ssh-keys/"+url.PathEscape(id), nil, url.Values{"label": {label}})
	return err
}

func (c *Client) DeleteSSHKey(ctx context.Context, id string) error {
	_, _, err := c.do(ctx, http.MethodDelete, "/client-ssh-keys/"+url.PathEscape(id), nil, nil)
	return err
}

// --- Dedicated servers ---

func (c *Client) ListDedicateds(ctx context.Context) ([]Object, error) {
	return c.list(ctx, "/dedicateds", nil)
}

func (c *Client) GetDedicated(ctx context.Context, hostname string) (Object, error) {
	return c.get(ctx, "/dedicateds/"+url.PathEscape(hostname))
}

type ReinstallOptions struct {
	TemplateID      int64
	RaidValue       string
	StartupScriptID int64
}

func (c *Client) ReinstallDedicated(ctx context.Context, hostname string, opts ReinstallOptions) error {
	form := url.Values{}
	if opts.TemplateID != 0 {
		form.Set("template_id", strconv.FormatInt(opts.TemplateID, 10))
	}
	if opts.RaidValue != "" {
		form.Set("raid_value", opts.RaidValue)
	}
	if opts.StartupScriptID != 0 {
		form.Set("startup_script_id", strconv.FormatInt(opts.StartupScriptID, 10))
	}
	return c.dedicatedAction(ctx, hostname, "reinstall", form)
}

// PowerDedicated runs one of: start, stop, restart.
func (c *Client) PowerDedicated(ctx context.Context, hostname, action string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("unknown power action %q", action)
	}
	return c.dedicatedAction(ctx, hostname, action, url.Values{})
}

// OrderKVM requests KVM console access. Access details are not returned by the API.
// clientIP restricts access to that IP when set.
func (c *Client) OrderKVM(ctx context.Context, hostname, clientIP string, whitelabel bool) error {
	form := url.Values{}
	if clientIP != "" {
		form.Set("client_ip", clientIP)
	}
	if whitelabel {
		form.Set("whitelabel", "1")
	}
	return c.dedicatedAction(ctx, hostname, "order-kvm", form)
}

func (c *Client) dedicatedAction(ctx context.Context, hostname, action string, form url.Values) error {
	_, _, err := c.do(ctx, http.MethodPost, "/dedicateds/"+url.PathEscape(hostname)+"/"+action, nil, form)
	return err
}

// --- Object helpers ---

// String returns a field as string; numbers and bools are formatted, nested values JSON-encoded.
func (o Object) String(key string) string {
	return Stringify(o[key])
}

func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// Int64 returns a numeric field, 0 if missing or not numeric.
func (o Object) Int64(key string) int64 {
	n, _ := strconv.ParseInt(o.String(key), 10, 64)
	return n
}

// Flatten converts all fields to strings, for exposing undocumented schemas.
func (o Object) Flatten() map[string]string {
	out := make(map[string]string, len(o))
	for k, v := range o {
		// _links are HAL navigation, not server data.
		if v == nil || k == "_links" {
			continue
		}
		out[k] = Stringify(v)
	}
	return out
}

// KeyMaterial returns the "<type> <base64>" part of an OpenSSH public key, ignoring the
// comment and whitespace, so differently formatted copies of the same key compare equal.
func KeyMaterial(v string) string {
	f := strings.Fields(v)
	if len(f) > 2 {
		f = f[:2]
	}
	return strings.Join(f, " ")
}
