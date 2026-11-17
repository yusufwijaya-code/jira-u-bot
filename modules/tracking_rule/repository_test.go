package tracking_rule

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"nudge/base/testutil"
	"nudge/modules/jira_client/jira_client_dto"
)

func setupRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenTestDB(t)
	cleanup := func() { db.Exec(`DELETE FROM ms_tracking_rule WHERE projectKey LIKE 'ZZTRT%'`) }
	cleanup()
	t.Cleanup(cleanup)
	return NewRepository(db)
}

// AC4.3, atomically: concurrent deactivations (locking the active rows) both
// succeed while other rules stay active and never leave zero active rules.
// Only ZZTRT rows are touched; the seeded rules keep the set non-empty, so
// the last-active branch is checked only if they happen to be inactive.
func TestRepository_ConcurrentDeactivationsKeepOneActive(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	for _, key := range []string{"ZZTRT1", "ZZTRT2"} {
		if err := repo.Insert(ctx, Rule{ProjectKey: key, ProjectName: key, TrackedStatuses: []string{"A"}, DoneStatus: "B",
			StoryPointFields: []string{"customfield_1"}, StoryPointWriteField: "customfield_1", StoryPointWriteFieldSubtask: "customfield_1",
			IsActive: true, EditedBy: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	errs := make(chan error, 2)
	for _, key := range []string{"ZZTRT1", "ZZTRT2"} {
		go func(k string) { errs <- repo.SetActive(ctx, k, false, "t") }(key)
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("SetActive: %v (other active rules exist, both may switch off)", err)
		}
	}
	rules, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var active []string
	for _, r := range rules {
		if r.IsActive {
			active = append(active, r.ProjectKey)
		}
	}
	if len(active) == 0 {
		t.Fatal("zero active rules")
	}
	if len(active) == 1 {
		if err := repo.SetActive(ctx, active[0], false, "t"); !errors.Is(err, ErrLastActiveRule) {
			t.Fatalf("last active err = %v, want ErrLastActiveRule", err)
		}
	}
}

// AC1.1/AC1.2: the seeded rows are exactly the builtin rules the code used.
func TestRepository_SeedEqualsTheBuiltinRules(t *testing.T) {
	rules, err := setupRepo(t).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Rule{}
	for _, r := range rules {
		byKey[r.ProjectKey] = r
	}
	for _, want := range jira_client_dto.BuiltinRules() {
		got, ok := byKey[want.ProjectKey]
		if !ok || !got.IsActive || !reflect.DeepEqual(got.Tracking(), want) {
			t.Errorf("%s = %+v, want %+v", want.ProjectKey, got.Tracking(), want)
		}
	}
}

func TestRepository_InsertFindUpdateSetActive(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	rule := Rule{
		ProjectKey: "ZZTRT", ProjectName: "Test", TrackedStatuses: []string{"In Progress", "Review"}, DoneStatus: "Done", ReviewStatus: "Review",
		StoryPointFields: []string{"customfield_1", "customfield_2"}, StoryPointWriteField: "customfield_1", StoryPointWriteFieldSubtask: "customfield_2",
		CreateFields: map[string][]string{"10206": {"duedate", "priority"}},
		IsActive:     true, EditedBy: "admin@esb.co.id",
	}
	if err := repo.Insert(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, rule); !errors.Is(err, ErrRuleExists) {
		t.Fatalf("second insert err = %v, want ErrRuleExists", err)
	}
	got, found, err := repo.Find(ctx, "ZZTRT")
	got.EditedDate = rule.EditedDate
	if err != nil || !found || !reflect.DeepEqual(got, rule) {
		t.Fatalf("Find = %+v, %v, %v", got, found, err)
	}

	rule.ProjectName, rule.TrackedStatuses, rule.ReviewStatus, rule.EditedBy = "Test 2", []string{"In Progress"}, "", "other@esb.co.id"
	rule.CreateFields = nil
	if err := repo.Update(ctx, rule); err != nil {
		t.Fatal(err)
	}
	got, _, _ = repo.Find(ctx, "ZZTRT")
	if got.CreateFields != nil {
		t.Fatalf("createFields after clearing = %v, want nil (NULL)", got.CreateFields)
	}
	if got.ProjectName != "Test 2" || got.ReviewStatus != "" || len(got.TrackedStatuses) != 1 || got.EditedBy != "other@esb.co.id" || !got.IsActive {
		t.Fatalf("after update = %+v", got)
	}

	if err := repo.SetActive(ctx, "ZZTRT", false, "x@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = repo.Find(ctx, "ZZTRT")
	if got.IsActive || got.EditedBy != "x@esb.co.id" {
		t.Fatalf("after deactivate = %+v", got)
	}
	if err := repo.SetActive(ctx, "ZZTRT", false, "y@esb.co.id"); err != nil {
		t.Fatalf("deactivating an inactive rule: %v", err)
	}
	if err := repo.SetActive(ctx, "ZZTRT", true, "x@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ = repo.Find(ctx, "ZZTRT"); !got.IsActive {
		t.Fatalf("after reactivate = %+v", got)
	}
	if _, found, err := repo.Find(ctx, "ZZTRT_NONE"); found || err != nil {
		t.Fatalf("unknown key: found=%v err=%v", found, err)
	}
}
