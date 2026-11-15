package user_setting_dto

import (
	"fmt"
	"errors"
	"reflect"
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func TestDefault_ReproducesLegacyBehavior(t *testing.T) {
	d := Default()
	if !reflect.DeepEqual(d.ReminderTiers, []string{"2h", "1h", "30m", "15m", "5m", "1m"}) {
		t.Errorf("ReminderTiers = %v, want the legacy 6 tiers", d.ReminderTiers)
	}
	if len(d.OverageTiers) != 0 {
		t.Errorf("OverageTiers = %v, want none", d.OverageTiers)
	}
	if d.OverageRepeatMinutes == nil || *d.OverageRepeatMinutes != 5 {
		t.Errorf("OverageRepeatMinutes = %v, want 5", d.OverageRepeatMinutes)
	}
	if !d.NotifyStatusChange {
		t.Error("both event notifications must default to on")
	}
	if d.IsSilenced(time.Now()) {
		t.Error("default settings must never be silenced")
	}
}

func TestNormalizeTiers_CanonicalizesAndSortsLargestFirst(t *testing.T) {
	got, err := NormalizeTiers([]string{"15m", "90m", "2H", " 45m ", "60m"})
	if err != nil {
		t.Fatalf("NormalizeTiers returned error: %v", err)
	}
	want := []string{"2h", "90m", "1h", "45m", "15m"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeTiers_EmptyIsAllowed(t *testing.T) {
	got, err := NormalizeTiers(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("NormalizeTiers(nil) = %v, %v; want empty, nil", got, err)
	}
}

func TestNormalizeTiers_Rejects(t *testing.T) {
	cases := map[string][]string{
		"bad format":         {"abc"},
		"no unit":            {"15"},
		"zero":               {"0m"},
		"above 8 hours":      {"9h"},
		"above 8h in min":    {"481m"},
		"duplicate same":     {"15m", "15m"},
		"duplicate 60m / 1h": {"60m", "1h"},
		"more than 10":       {"1m", "2m", "3m", "4m", "5m", "6m", "7m", "8m", "9m", "10m", "11m"},
	}
	for name, codes := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeTiers(codes); err == nil {
				t.Errorf("NormalizeTiers(%v) = nil error, want a validation error", codes)
			}
		})
	}
}

func TestSplitTierCSV_DropsInvalidEntriesInsteadOfFailing(t *testing.T) {
	got := SplitTierCSV("1h,bogus,,30m,99h,30m")
	if !reflect.DeepEqual(got, []string{"1h", "30m"}) {
		t.Errorf("got %v, want [1h 30m]", got)
	}
}

func TestValidateOverageRepeat(t *testing.T) {
	for _, ok := range []*int{nil, intPtr(5), intPtr(10), intPtr(15), intPtr(30), intPtr(60)} {
		if err := ValidateOverageRepeat(ok); err != nil {
			t.Errorf("ValidateOverageRepeat(%v) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []int{0, 1, 7, 45, 120} {
		if err := ValidateOverageRepeat(intPtr(bad)); err == nil {
			t.Errorf("ValidateOverageRepeat(%d) = nil, want error", bad)
		}
	}
}

func TestNormalizeQuietHours(t *testing.T) {
	got, err := NormalizeQuietHours(&QuietHours{Start: "9:5", End: "17:30"})
	if err != nil || got.Start != "09:05" || got.End != "17:30" {
		t.Errorf("got %+v, %v; want 09:05-17:30", got, err)
	}
	if got, err := NormalizeQuietHours(nil); got != nil || err != nil {
		t.Errorf("nil quiet hours must stay disabled, got %+v, %v", got, err)
	}
	for _, bad := range []QuietHours{{Start: "12:00", End: ""}, {Start: "", End: "13:00"}, {Start: "25:00", End: "13:00"}, {Start: "12:00", End: "12:00"}} {
		bad := bad
		if _, err := NormalizeQuietHours(&bad); err == nil {
			t.Errorf("NormalizeQuietHours(%+v) = nil error, want error", bad)
		}
	}
}

func wib(hour, minute int) time.Time {
	return time.Date(2026, 11, 16, hour, minute, 0, 0, Jakarta)
}

func TestInQuietHours_SameDayWindow(t *testing.T) {
	s := Default()
	s.QuietHours = &QuietHours{Start: "12:00", End: "13:00"}
	if !s.InQuietHours(wib(12, 0)) || !s.InQuietHours(wib(12, 59)) {
		t.Error("12:00 and 12:59 must be inside 12:00-13:00")
	}
	if s.InQuietHours(wib(11, 59)) || s.InQuietHours(wib(13, 0)) {
		t.Error("11:59 and 13:00 must be outside 12:00-13:00 (end is exclusive)")
	}
}

func TestInQuietHours_WindowCrossingMidnight(t *testing.T) {
	s := Default()
	s.QuietHours = &QuietHours{Start: "22:00", End: "06:00"}
	if !s.InQuietHours(wib(23, 30)) || !s.InQuietHours(wib(5, 59)) {
		t.Error("23:30 and 05:59 must be inside 22:00-06:00")
	}
	if s.InQuietHours(wib(6, 0)) || s.InQuietHours(wib(12, 0)) {
		t.Error("06:00 and 12:00 must be outside 22:00-06:00")
	}
}

func TestInQuietHours_EvaluatesInJakartaRegardlessOfInputZone(t *testing.T) {
	s := Default()
	s.QuietHours = &QuietHours{Start: "12:00", End: "13:00"}
	utcNoonWIB := time.Date(2026, 11, 16, 5, 30, 0, 0, time.UTC) // 12:30 WIB
	if !s.InQuietHours(utcNoonWIB) {
		t.Error("05:30 UTC is 12:30 WIB and must be inside quiet hours")
	}
}

func TestIsMuted(t *testing.T) {
	now := wib(10, 0)
	s := Default()
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)

	s.MutedUntil = &future
	if !s.IsMuted(now) || !s.IsSilenced(now) {
		t.Error("a mute ending in the future must silence")
	}
	s.MutedUntil = &past
	if s.IsMuted(now) || s.IsSilenced(now) {
		t.Error("an expired mute must not silence")
	}
}

func TestTomorrowMorning(t *testing.T) {
	got := TomorrowMorning(time.Date(2026, 11, 14, 23, 50, 0, 0, Jakarta))
	want := time.Date(2026, 11, 15, 8, 30, 0, 0, Jakarta)
	if !got.Equal(want) {
		t.Errorf("TomorrowMorning = %v, want %v", got, want)
	}
}

// Clicked in the small hours, "besok" means the coming morning, not the
// morning after it (review finding #8).
func TestTomorrowMorning_AfterMidnightMeansSameDayMorning(t *testing.T) {
	got := TomorrowMorning(time.Date(2026, 11, 15, 0, 30, 0, 0, Jakarta))
	want := time.Date(2026, 11, 15, 8, 30, 0, 0, Jakarta)
	if !got.Equal(want) {
		t.Errorf("TomorrowMorning(00:30) = %v, want %v", got, want)
	}
}

func TestTomorrowMorning_FromFourAMIsNextDay(t *testing.T) {
	got := TomorrowMorning(time.Date(2026, 11, 15, 4, 0, 0, 0, Jakarta))
	want := time.Date(2026, 11, 16, 8, 30, 0, 0, Jakarta)
	if !got.Equal(want) {
		t.Errorf("TomorrowMorning(04:00) = %v, want %v", got, want)
	}
}

func TestApply_ValidInputKeepsMuteAndUserID(t *testing.T) {
	muted := wib(15, 0)
	base := Default()
	base.UserID = 7
	base.MutedUntil = &muted

	got, err := NotificationInput{
		ReminderTiers:        []string{"45m", "2h"},
		OverageTiers:         []string{"1h", "15m"},
		OverageRepeatMinutes: nil,
		NotifyStatusChange:   false,
		QuietHours:           &QuietHours{Start: "12:00", End: "13:00"},
	}.Apply(base)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.UserID != 7 || got.MutedUntil != &muted {
		t.Error("Apply must keep UserID and MutedUntil from base")
	}
	if !reflect.DeepEqual(got.ReminderTiers, []string{"2h", "45m"}) || !reflect.DeepEqual(got.OverageTiers, []string{"1h", "15m"}) {
		t.Errorf("tiers = %v / %v", got.ReminderTiers, got.OverageTiers)
	}
	if got.OverageRepeatMinutes != nil || got.NotifyStatusChange || got.QuietHours.Start != "12:00" {
		t.Errorf("unexpected result %+v", got)
	}
}

func TestApply_ReportsTheInvalidField(t *testing.T) {
	cases := map[string]NotificationInput{
		"reminderTiers":        {ReminderTiers: []string{"9h"}},
		"overageTiers":         {OverageTiers: []string{"x"}},
		"overageRepeatMinutes": {OverageRepeatMinutes: intPtr(7)},
		"quietHours":           {QuietHours: &QuietHours{Start: "12:00"}},
	}
	for field, in := range cases {
		t.Run(field, func(t *testing.T) {
			_, err := in.Apply(Default())
			var verr *ValidationError
			if !errors.As(err, &verr) || verr.Field != field {
				t.Errorf("err = %v, want ValidationError on %s", err, field)
			}
		})
	}
}

func TestMuteForever_SilencesAndIsDetected(t *testing.T) {
	s := Default()
	forever := MuteForeverUntil
	s.MutedUntil = &forever
	if !s.IsMutedForever() || !s.IsSilenced(time.Date(2099, 1, 1, 10, 0, 0, 0, Jakarta)) {
		t.Error("a forever mute must be detected and silence any future moment")
	}
	oneHour := time.Now().Add(time.Hour)
	s.MutedUntil = &oneHour
	if s.IsMutedForever() {
		t.Error("a regular mute must not count as forever")
	}
}

// Tech Spec "Notifikasi Perubahan Status Tiket".
func TestNormalizeStatuses_TrimsAndDropsDuplicates(t *testing.T) {
	got, err := NormalizeStatuses([]string{" Code Review ", "", "code review", "Done"})
	if err != nil || !reflect.DeepEqual(got, []string{"Code Review", "Done"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := NormalizeStatuses([]string{"A,B"}); err == nil {
		t.Fatal("a comma must be rejected (stored as CSV)")
	}
}

func TestWantsStatusChange(t *testing.T) {
	all := UserSetting{NotifyStatusChange: true}
	if !all.WantsStatusChange("SIT") {
		t.Error("an empty filter means every status")
	}
	some := UserSetting{NotifyStatusChange: true, StatusChangeStatuses: []string{"Code Review"}}
	if !some.WantsStatusChange("CODE REVIEW") || some.WantsStatusChange("To Do") {
		t.Error("the filter matches destination statuses case-insensitively")
	}
	off := UserSetting{NotifyStatusChange: false}
	if off.WantsStatusChange("Done") {
		t.Error("the toggle off means nothing")
	}
}

func TestApply_RejectsInvalidStatusFilter(t *testing.T) {
	_, err := NotificationInput{StatusChangeStatuses: []string{"A,B"}}.Apply(Default())
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "statusChangeStatuses" {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizeStatuses_RejectsAListLongerThanTheColumn(t *testing.T) {
	var long []string
	for i := 0; i < 20; i++ {
		long = append(long, fmt.Sprintf("Status nomor %02d yang cukup panjang", i))
	}
	if _, err := NormalizeStatuses(long); err == nil {
		t.Fatal("a filter longer than the column must be rejected")
	}
}
