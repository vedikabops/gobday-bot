package models

import "time"

type Birthday struct {
	ID      int64  `bun:",pk,autoincrement"`
	GroupID int64  `bun:"group_id,notnull"`
	Name    string `bun:"name,notnull"`
	Day     int    `bun:"day,notnull"`
	Month   int    `bun:"month,notnull"`
}

type GroupSettings struct {
	GroupID   int64     `bun:"group_id,pk"`
	Enabled   bool      `bun:",default:true"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
