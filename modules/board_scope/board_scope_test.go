package board_scope

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/base/testutil"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/registration/registration_repository"
)

const testEmail = "testmysql-boardscope@esb.co.id"

func TestRepository_GetSetRoundTrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	if err := registration_repository.NewRegistrationRepository(db).UpsertUser(ctx, "TestMySQL BoardScope", testEmail, "acc-boardscope"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE email = ?`, testEmail) })
	r := New(db)

	if id, err := r.Get(ctx, testEmail); err != nil || id != 0 {
		t.Fatalf("new user: id %d err %v, want 0 (every board)", id, err)
	}
	if err := r.Set(ctx, "TestMySQL-BoardScope@esb.co.id", 26); err != nil {
		t.Fatal(err)
	}
	if id, _ := r.Get(ctx, testEmail); id != 26 {
		t.Errorf("after set: %d, want 26", id)
	}
	if err := r.Set(ctx, testEmail, 0); err != nil {
		t.Fatal(err)
	}
	if id, _ := r.Get(ctx, testEmail); id != 0 {
		t.Errorf("after clear: %d, want 0", id)
	}
	if id, err := r.Get(ctx, "nobody@esb.co.id"); err != nil || id != 0 {
		t.Errorf("unknown user: %d %v", id, err)
	}
}

type memStore map[string]int64

func (m memStore) Get(_ context.Context, email string) (int64, error) { return m[email], nil }
func (m memStore) Set(_ context.Context, email string, id int64) error {
	m[email] = id
	return nil
}

func call(h *Handler, method, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/api/dashboard/board-scope", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{Email: "dev@esb.co.id"})
	if method == http.MethodGet {
		h.Get(c)
	} else {
		h.Put(c)
	}
	return w
}

func TestHandler_PutAndGet(t *testing.T) {
	store := memStore{}
	h := NewHandler(store)
	if w := call(h, http.MethodPut, `{"boardId":28}`); w.Code != 200 || store["dev@esb.co.id"] != 28 || w.Body.String() != `{"boardId":28}` {
		t.Fatalf("put: %d %s %v", w.Code, w.Body, store)
	}
	if w := call(h, http.MethodGet, ""); w.Body.String() != `{"boardId":28}` {
		t.Errorf("get: %s", w.Body)
	}
	if w := call(h, http.MethodPut, `{"boardId":null}`); w.Code != 200 || store["dev@esb.co.id"] != 0 || w.Body.String() != `{"boardId":null}` {
		t.Errorf("clear: %d %s", w.Code, w.Body)
	}
	if w := call(h, http.MethodPut, `{"boardId":-1}`); w.Code != 400 {
		t.Errorf("negative id: %d", w.Code)
	}
}
