package chat_log_repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"nudge/base/testutil"
	"nudge/modules/chat_log/chat_log_dto"
)

const testEmail = "testmysql-chatlog@esb.co.id"

func setup(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenTestDB(t)
	clean := func() { db.Exec(`DELETE FROM tr_chatlog WHERE email = ?`, testEmail) }
	clean()
	t.Cleanup(clean)
	return New(db)
}

func TestInsertAndList_NewestFirstWithPaging(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	for _, body := range []string{"satu", "dua", "tiga"} {
		r.Record(ctx, chat_log_dto.Entry{Email: "TestMySQL-ChatLog@esb.co.id", Direction: "out", Kind: "message", Channel: "app", Status: "sent", Body: body})
	}

	page, err := r.List(ctx, chat_log_dto.Filter{Email: testEmail, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Body != "tiga" || page.Items[1].Body != "dua" || page.NextBeforeID == 0 {
		t.Fatalf("page 1 = %+v", page)
	}
	if page.Items[0].Email != testEmail {
		t.Errorf("email not lower-cased: %q", page.Items[0].Email)
	}

	page, err = r.List(ctx, chat_log_dto.Filter{Email: testEmail, Limit: 2, BeforeID: page.NextBeforeID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Body != "satu" || page.NextBeforeID != 0 {
		t.Fatalf("page 2 = %+v", page)
	}
}

func TestList_DateRangeAndTruncation(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	r.Record(ctx, chat_log_dto.Entry{Email: testEmail, Direction: "in", Kind: "command", Status: "received", Body: strings.Repeat("x", 5000)})

	page, err := r.List(ctx, chat_log_dto.Filter{Email: testEmail, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("in range: %+v %v", page, err)
	}
	if n := len([]rune(page.Items[0].Body)); n != chat_log_dto.MaxBodyRunes {
		t.Errorf("body runes = %d", n)
	}
	page, _ = r.List(ctx, chat_log_dto.Filter{Email: testEmail, To: time.Now().Add(-time.Hour)})
	if len(page.Items) != 0 {
		t.Errorf("out of range: %+v", page)
	}
}

func TestPurgeBefore(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	r.Record(ctx, chat_log_dto.Entry{Email: testEmail, Direction: "out", Kind: "message", Status: "sent", Body: "lama"})
	if _, err := r.db.Exec(`UPDATE tr_chatlog SET createdDate = ? WHERE email = ?`, time.Now().AddDate(0, 0, -100), testEmail); err != nil {
		t.Fatal(err)
	}
	r.Record(ctx, chat_log_dto.Entry{Email: testEmail, Direction: "out", Kind: "message", Status: "sent", Body: "baru"})

	if _, err := r.PurgeBefore(ctx, time.Now().AddDate(0, 0, -90)); err != nil {
		t.Fatal(err)
	}
	page, _ := r.List(ctx, chat_log_dto.Filter{Email: testEmail})
	if len(page.Items) != 1 || page.Items[0].Body != "baru" {
		t.Fatalf("after purge = %+v", page)
	}
}
