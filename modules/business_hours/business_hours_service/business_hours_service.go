package business_hours_service

import (
	"time"

	"jira-u-bot/modules/business_hours/business_hours_dto"
)

type BusinessHoursService interface {
	ElapsedBusinessSeconds(start, end time.Time, cal business_hours_dto.HolidayCalendar) int64
	LoadHolidayCalendar(path string) (business_hours_dto.HolidayCalendar, error)
}
