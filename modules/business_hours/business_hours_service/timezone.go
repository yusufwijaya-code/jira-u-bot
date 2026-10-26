package business_hours_service

import (
	"time"

	_ "time/tzdata"
)

// jakartaLocation is loaded once. The blank import of time/tzdata embeds the
// full IANA database into the binary so this never fails due to a missing
// host tzdata (e.g. a minimal container image) — spec/plan global constraint:
// business hours are "stored and compared in Asia/Jakarta, never server-local time."
var jakartaLocation = mustLoadJakarta()

func mustLoadJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// WIB has no DST, so a fixed +07:00 offset is wall-clock-identical to
		// Asia/Jakarta for every calculation this package does.
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}
