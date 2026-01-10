package models

type Birthday struct {
	ID      int64  `bun:",pk,autoincrement"`
	GroupID int64  `bun:"group_id,notnull"`
	Name    string `bun:"name,notnull"`
	Day     int    `bun:"day,notnull"`
	Month   int    `bun:"month,notnull"`
}
