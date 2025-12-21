package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID            uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" validate:"-"`
	TelegramID    int64     `bun:"telegram_id,unique,notnull" validate:"required"`
	Username      *string   `bun:"username" validate:"omitempty"`
	FirstName     string    `bun:"first_name,notnull" validate:"required"`
	LastName      *string   `bun:"last_name" validate:"omitempty"`
	BirthdayDay   int       `bun:"birthday_day,notnull" validate:"required,min=1,max=31"`
	BirthdayMonth int       `bun:"birthday_month,notnull" validate:"required,min=1,max=12"`
	BirthdayYear  *int      `bun:"birthday_year" validate:"omitempty,min=1900,max=2100"`
	Timezone      *string   `bun:"timezone" validate:"omitempty"`
	IsActive      bool      `bun:"is_active,notnull,default:true" validate:"-"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp" validate:"-"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:current_timestamp" validate:"-"`
}

// groups where the user wants birthday reminders
type RegisteredGroup struct {
	UserID  uuid.UUID `bun:",pk,type:uuid" validate:"-"`
	GroupID int64     `bun:",pk,notnull" validate:"required"` // Telegram chat ID (negative for groups)
	AddedAt time.Time `bun:"added_at,notnull,default:current_timestamp" validate:"-"`
}
