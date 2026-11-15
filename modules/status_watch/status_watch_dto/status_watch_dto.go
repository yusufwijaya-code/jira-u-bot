package status_watch_dto

import "time"

// Snapshot is the last status the status watcher saw for one ticket (Tech
// Spec "Notifikasi Perubahan Status Tiket").
type Snapshot struct {
	IssueKey      string
	Status        string
	AssigneeEmail string
	LastSeen      time.Time
}
