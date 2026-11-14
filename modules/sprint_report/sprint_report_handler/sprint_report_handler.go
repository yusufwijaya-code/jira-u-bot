package sprint_report_handler

import "github.com/gin-gonic/gin"

type SprintReportHandler interface {
	// GetReport returns the Sprint Report page's data as JSON — available
	// sprints always included, generated report included only when at least
	// one sprint was selected via query param (Tech Spec "Angular Dashboard
	// Revamp", Repository 1 Chapter 6).
	GetReport(c *gin.Context)
	// Backfill rebuilds the caller's untracked active-sprint time from
	// Jira's changelog (Tech Spec "Active Sprint Tracking & Changelog
	// Backfill", AC3.7) — the Sprint card's "Hitung ulang dari Jira".
	Backfill(c *gin.Context)
}
