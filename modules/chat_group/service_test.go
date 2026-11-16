package chat_group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeAdder struct {
	errs  map[string]error
	added []string
}

func (f *fakeAdder) AddMember(_ context.Context, email string) error {
	f.added = append(f.added, email)
	return f.errs[email]
}

type fakeStore struct {
	needs   map[string]bool
	pending []string
	marked  []string
	failed  map[string]string
}

func (f *fakeStore) NeedsJoin(_ context.Context, email string) (bool, error) { return f.needs[email], nil }
func (f *fakeStore) Pending(_ context.Context, limit int) ([]string, error) {
	if len(f.pending) > limit {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}
func (f *fakeStore) MarkAdded(_ context.Context, email string) error {
	f.marked = append(f.marked, email)
	return nil
}
func (f *fakeStore) MarkFailed(_ context.Context, email, msg string) error {
	if f.failed == nil {
		f.failed = map[string]string{}
	}
	f.failed[email] = msg
	return nil
}

func TestEnsureMember_AddsOnlyWhenNeeded(t *testing.T) {
	a := &fakeAdder{}
	st := &fakeStore{needs: map[string]bool{"new@esb.co.id": true}}
	svc := NewService(a, st)
	svc.EnsureMember(context.Background(), "new@esb.co.id")
	svc.EnsureMember(context.Background(), "done@esb.co.id")
	if strings.Join(a.added, ",") != "new@esb.co.id" || strings.Join(st.marked, ",") != "new@esb.co.id" {
		t.Errorf("added=%v marked=%v, want only the user that needs it", a.added, st.marked)
	}
}

func TestEnsureMember_RecordsAShortenedError(t *testing.T) {
	long := strings.Repeat("é", 400)
	a := &fakeAdder{errs: map[string]error{"new@esb.co.id": errors.New(long)}}
	st := &fakeStore{needs: map[string]bool{"new@esb.co.id": true}}
	NewService(a, st).EnsureMember(context.Background(), "new@esb.co.id")
	msg, ok := st.failed["new@esb.co.id"]
	if len(st.marked) != 0 || !ok || utf8.RuneCountInString(msg) != maxErrLen || !utf8.ValidString(msg) {
		t.Errorf("marked=%v failed=%q, want one valid error of %d characters", st.marked, msg, maxErrLen)
	}
}

func TestEnsureMember_UnauthorizedBlamesTheTokenNotTheUser(t *testing.T) {
	a := &fakeAdder{errs: map[string]error{"new@esb.co.id": ErrUnauthorized}}
	st := &fakeStore{needs: map[string]bool{"new@esb.co.id": true}}
	NewService(a, st).EnsureMember(context.Background(), "new@esb.co.id")
	if len(st.failed) != 0 || len(st.marked) != 0 {
		t.Errorf("failed=%v marked=%v, want nothing recorded", st.failed, st.marked)
	}
}

func TestSweep_CapsAtFifty(t *testing.T) {
	var pending []string
	for i := 0; i < 60; i++ {
		pending = append(pending, fmt.Sprintf("u%d@esb.co.id", i))
	}
	a := &fakeAdder{}
	svc := NewService(a, &fakeStore{pending: pending})
	svc.pause = 0
	if n := svc.Sweep(context.Background()); n != 50 || len(a.added) != 50 {
		t.Fatalf("sweep = %d added %d, want 50", n, len(a.added))
	}
}

func TestSweep_StopsOnUnauthorizedAndRecordsOtherFailures(t *testing.T) {
	pending := []string{"a@esb.co.id", "b@esb.co.id", "c@esb.co.id", "d@esb.co.id"}
	a := &fakeAdder{errs: map[string]error{"b@esb.co.id": errors.New("HTTP 403"), "c@esb.co.id": ErrUnauthorized}}
	st := &fakeStore{pending: pending}
	svc := NewService(a, st)
	svc.pause = 0
	n := svc.Sweep(context.Background())
	if n != 1 || strings.Join(a.added, ",") != "a@esb.co.id,b@esb.co.id,c@esb.co.id" {
		t.Errorf("sweep = %d added=%v, want stop at the 401", n, a.added)
	}
	if _, ok := st.failed["b@esb.co.id"]; !ok || len(st.failed) != 1 {
		t.Errorf("failed=%v, want only b recorded", st.failed)
	}
}

func TestSweep_GroupConfigErrorStopsAndBlamesNobody(t *testing.T) {
	cfgErr := fmt.Errorf("%w: lookup: HTTP 404", ErrGroupConfig)
	a := &fakeAdder{errs: map[string]error{"a@esb.co.id": cfgErr, "b@esb.co.id": cfgErr}}
	st := &fakeStore{pending: []string{"a@esb.co.id", "b@esb.co.id"}}
	svc := NewService(a, st)
	svc.pause = 0
	if n := svc.Sweep(context.Background()); n != 0 || len(a.added) != 1 || len(st.failed) != 0 {
		t.Errorf("sweep = %d added=%v failed=%v, want one try, nothing recorded", n, a.added, st.failed)
	}
}
