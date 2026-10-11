package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListPaginatesAndAuthenticates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access-token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"name":"Unauthorized","message":"bad token","status":401}`)
			return
		}
		if r.URL.Query().Get("type") != "dedicated" {
			t.Errorf("type filter missing")
		}
		w.Header().Set("X-Pagination-Page-Count", "2")
		fmt.Fprintf(w, `[{"id":%s,"name":"Debian","exists":1}]`, r.URL.Query().Get("page"))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "tok").ListTemplates(context.Background(), "dedicated")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Int64("id") != 2 || got[0].String("name") != "Debian" {
		t.Fatalf("unexpected: %#v", got)
	}

	_, err = New(srv.URL, "wrong").ListTemplates(context.Background(), "dedicated")
	if err == nil || err.Error() != "mevspace api: HTTP 401: bad token" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCreateSSHKeyFallsBackToList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if err := r.ParseForm(); err != nil || r.PostForm.Get("label") != "me" {
				t.Errorf("bad form: %v %v", err, r.PostForm)
			}
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `"Ssh key created"`)
		case http.MethodGet:
			fmt.Fprint(w, `[{"id":1,"label":"other","value":"x"},{"id":7,"label":"me","value":"ssh-ed25519 AAA"}]`)
		}
	}))
	defer srv.Close()

	k, err := New(srv.URL, "tok").CreateSSHKey(context.Background(), "me", "ssh-ed25519 AAA\n")
	if err != nil {
		t.Fatal(err)
	}
	if k.String("id") != "7" {
		t.Fatalf("got id %q", k.String("id"))
	}
}

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Object not found: 3"}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "tok").GetSSHKey(context.Background(), "3")
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestNetworkErrorRedactsToken(t *testing.T) {
	_, err := New("http://127.0.0.1:1", "supersecret").ListSSHKeys(context.Background())
	if err == nil || strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("token leaked or no error: %v", err)
	}
}

func TestRequestBuildErrorRedactsToken(t *testing.T) {
	tok := "s3cr/t+tok"
	_, err := New("https://bad host", tok).ListSSHKeys(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, leak := range []string{tok, url.QueryEscape(tok)} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("token leaked: %v", err)
		}
	}
}

func TestAPIErrorStaysTypedAndRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"message":"no route for %s"}`, r.URL.RawQuery)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "tok123").GetSSHKey(context.Background(), "1")
	if !IsNotFound(err) || strings.Contains(err.Error(), "tok123") {
		t.Fatalf("got %v", err)
	}
}

func TestEndpointMustBeHTTPS(t *testing.T) {
	_, err := New("http://api.example.com", "tok").ListSSHKeys(context.Background())
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("got %v", err)
	}
}

func TestCrossHostRedirectRefused(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("token followed redirect to other host: %s", r.URL.RawQuery)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+r.URL.RequestURI(), http.StatusFound)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "tok123").ListSSHKeys(context.Background())
	if err == nil || strings.Contains(err.Error(), "tok123") {
		t.Fatalf("expected redacted redirect error, got %v", err)
	}
}

func TestKeyMaterialAndFlatten(t *testing.T) {
	if KeyMaterial("ssh-ed25519  AAAA me@host\n") != KeyMaterial("ssh-ed25519 AAAA") {
		t.Fatal("key material should ignore comment/whitespace")
	}
	if _, ok := (Object{"_links": map[string]any{"self": "x"}, "a": "b"}).Flatten()["_links"]; ok {
		t.Fatal("_links must be dropped")
	}
}
