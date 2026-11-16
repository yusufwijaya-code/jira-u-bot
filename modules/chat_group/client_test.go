package chat_group

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func fakeAPI(t *testing.T, memberStatus int, calls *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(body))
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		switch r.URL.Path {
		case "/groups:lookup":
			io.WriteString(w, `{"name":"groups/abc"}`)
		case "/groups/abc/memberships":
			w.WriteHeader(memberStatus)
			io.WriteString(w, `{"done":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testClient(url string) *Client {
	return NewClient(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"}), "nudge-users@esb.co.id", url)
}

func TestAddMember_LooksUpOnceThenAdds(t *testing.T) {
	var calls []string
	srv := fakeAPI(t, http.StatusOK, &calls)
	defer srv.Close()
	c := testClient(srv.URL)
	for _, e := range []string{"a@esb.co.id", "b@esb.co.id"} {
		if err := c.AddMember(context.Background(), e); err != nil {
			t.Fatalf("AddMember(%s) = %v", e, err)
		}
	}
	if len(calls) != 3 || !strings.Contains(calls[0], "groupKey.id=nudge-users%40esb.co.id") {
		t.Fatalf("calls = %v, want 1 lookup + 2 adds", calls)
	}
	if !strings.Contains(calls[1], `"preferredMemberKey":{"id":"a@esb.co.id"}`) || !strings.Contains(calls[1], `"roles":[{"name":"MEMBER"}]`) {
		t.Errorf("add body = %s", calls[1])
	}
}

func TestAddMember_ConflictMeansAlreadyMember(t *testing.T) {
	var calls []string
	srv := fakeAPI(t, http.StatusConflict, &calls)
	defer srv.Close()
	if err := testClient(srv.URL).AddMember(context.Background(), "a@esb.co.id"); err != nil {
		t.Fatalf("409 = %v, want nil", err)
	}
}

func TestAddMember_UnauthorizedAndOtherErrors(t *testing.T) {
	var calls []string
	srv := fakeAPI(t, http.StatusUnauthorized, &calls)
	err := testClient(srv.URL).AddMember(context.Background(), "a@esb.co.id")
	srv.Close()
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("401: err = %v, want ErrUnauthorized", err)
	}

	srv = fakeAPI(t, http.StatusForbidden, &calls)
	defer srv.Close()
	err = testClient(srv.URL).AddMember(context.Background(), "a@esb.co.id")
	if err == nil || errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "403") {
		t.Errorf("403: err = %v, want a non-auth error mentioning 403", err)
	}
}

func TestAddMember_GroupLookupForbiddenIsAConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	err := testClient(srv.URL).AddMember(context.Background(), "a@esb.co.id")
	if !errors.Is(err, ErrGroupConfig) {
		t.Errorf("lookup 403: err = %v, want ErrGroupConfig", err)
	}
}
