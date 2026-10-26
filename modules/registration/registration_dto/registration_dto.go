package registration_dto

import "time"

type User struct {
	UserID      int64
	UserName    string
	Email       string
	FlagActive  bool
	CreatedBy   string
	CreatedDate time.Time
}
