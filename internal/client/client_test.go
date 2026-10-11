package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
