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
		IsActive: true, EditedBy: "admin@esb.co.id",
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
	if err := repo.Update(ctx, rule); err != nil {
		t.Fatal(err)
	}
	got, _, _ = repo.Find(ctx, "ZZTRT")
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
	if _, found, err := repo.Find(ctx, "ZZTRT_NONE"); found || err != nil {
		t.Fatalf("unknown key: found=%v err=%v", found, err)
	}
}
