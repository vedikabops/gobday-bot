package main

import (
	"bdayBot/internal/user/models"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	tele "gopkg.in/telebot.v4"

	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

var IST *time.Location

func init() {
	var err error
	IST, err = time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// if the server does not have TZ data, fallback
		IST = time.FixedZone("IST", 5.5*3600)
	}
}

func createSchema(ctx context.Context, db *bun.DB) error {
	models := []interface{}{
		(*models.Birthday)(nil),
		(*models.GroupSettings)(nil),
	}

	for _, model := range models {
		_, err := db.NewCreateTable().
			Model(model).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	// load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// initialize database connection
	//pulling dsn
	dsn := os.Getenv("DB_DSN")
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())

	// create tables (the schema)
	ctx := context.Background()
	if err = createSchema(ctx, db); err != nil {
		log.Fatal("Failed to create database schema:", err)
	}

	//bot settings
	pref := tele.Settings{
		Token:  os.Getenv("TOKEN"),
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal(err)
		return
	}

	middleware := requireRemindersEnabled(db)

	// existing handler
	b.Handle("/hello", func(c tele.Context) error {
		return c.Send("Hello! I'm the Birthday Reminder Bot.")
	})

	b.Handle("/start", func(c tele.Context) error {
		return c.Send("Welcome to the Birthday Reminder Bot!\n\n" + "Use '/add [name] [DD-MM]` to save a birthday for this group.\n")
	})

	b.Handle("/help", func(c tele.Context) error {
		return c.Send(`*Birthday Reminder Bot Commands!*

*Setup and Information*
•/add [name] [DD-MM] - Add or update a birthday
•/list - List all saved birthdays
•/remove [name] - Remove a birthday

*Notification Control*
•/enable - Enable birthday reminders in this group
•/disable - Disable birthday reminders in this group

*General*
•/help - Show this help message

Note: Birthday Reminders are specific to this group only.`, tele.ModeMarkdown)
	})

	b.Handle("/add", middleware(func(c tele.Context) error {
		args := c.Args()
		if len(args) != 2 {
			return c.Send("Usage: /add [name] [DD-MM]")
		}

		name := args[0]
		dateStr := args[1]

		// parse date using a dummy year
		fullDateStr := fmt.Sprintf("%s-2024", dateStr)
		parsedDate, err := time.Parse("02-01-2006", fullDateStr)

		if err != nil {
			return c.Send("Invalid Date! Please use a valid date in DD-MM format.")
		}

		//now extract day and month
		d := parsedDate.Day()
		m := int(parsedDate.Month())

		// insert to table
		_, err = db.Exec("INSERT INTO birthdays (group_id, name, day, month) VALUES (?, ?, ?, ?) ON CONFLICT (group_id, name) DO UPDATE SET day = EXCLUDED.day, month = EXCLUDED.month", c.Chat().ID, name, d, m)

		if err != nil {
			log.Println("Add error:", err)
			return c.Send("Error saving birthday to database")
		}

		return c.Send(fmt.Sprintf("Added %s's birthday on %02d-%02d to list!", name, d, m))

	}))

	b.Handle("/list", middleware(func(c tele.Context) error {
		var bdays []models.Birthday

		// fetch from db
		err = db.NewSelect().
			Model(&bdays).
			Where("group_id=?", c.Chat().ID).
			Order("month ASC").
			Order("day ASC").
			Scan(context.Background())

		if err != nil {
			log.Println("List error:", err)
			return c.Send("Could not retrieve birthday list")
		}

		// check if empty
		if len(bdays) == 0 {
			return c.Send("no birthdays in list yet. Use /add to start!")
		}

		// reponse string
		var listMsg strings.Builder
		listMsg.WriteString("🎂 **Birthdays:**\n\n")

		for _, b := range bdays {
			listMsg.WriteString(fmt.Sprintf("- %s: %02d-%02d\n", b.Name, b.Day, b.Month))
		}

		return c.Send(listMsg.String())
	}))

	b.Handle("/remove", middleware(func(c tele.Context) error {
		args := c.Args()
		if len(args) != 1 {
			return c.Send("Usage: /remove [name]")
		}

		name := args[0]

		// delete where name and group_id match
		res, err := db.NewDelete().
			Model((*models.Birthday)(nil)).
			Where("group_id =?", c.Chat().ID).
			Where("name = ?", name).
			Exec(context.Background())

		if err != nil {
			log.Println("delete error:", err)
			return c.Send("error removing birthday.")
		}

		// check if row was actually deleted
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			return c.Send(fmt.Sprintf("I couldn't find anyone named '%s' in this group.", name))
		}

		return c.Send(fmt.Sprintf("Removed %s's birthday from list!", name))
	}))

	// handlers enable and disable (to turn reminders on/off)
	b.Handle("/enable", func(c tele.Context) error {
		_, err = db.NewInsert().
			Model(&models.GroupSettings{
				GroupID: c.Chat().ID,
				Enabled: true,
			}).
			On("CONFLICT (group_id) DO UPDATE SET enabled = EXCLUDED.enabled").
			Exec(context.Background())
		if err != nil {
			log.Println("Enable error:", err)
			return c.Send("Failed to enable reminders.")
		}

		return c.Send("Birthday Reminders are now enabled for this group!")
	})

	/*
		// why this didnt work: if db didnt recognize a column as a unique key, it didnt know there is a conflict
			b.Handle("/disable", func(c tele.Context) error {
				_, err := db.NewInsert().
					Model(&models.GroupSettings{
						GroupID: c.Chat().ID,
						Enabled: false,
					}).
					On("CONFLICT (group_id) DO UPDATE SET enabled = EXCLUDED.enabled").
					Exec(context.Background())

				if err != nil {
					log.Println("Disable error:", err)
					return c.Send("Failed to disable reminders.")
				}

				return c.Send("Birthday Reminders are now disabled for this group.")
			})
	*/

	b.Handle("/disable", func(c tele.Context) error {
		// Force a direct update to ensure Postgres sees it
		res, err := db.NewUpdate().
			Model(&models.GroupSettings{}).
			Set("enabled = ?", false).
			Where("group_id = ?", c.Chat().ID).
			Exec(context.Background())

		// If the row didn't exist yet, we need to Insert it instead
		rows, _ := res.RowsAffected()
		if rows == 0 {
			_, err = db.NewInsert().
				Model(&models.GroupSettings{
					GroupID: c.Chat().ID,
					Enabled: false,
				}).
				Exec(context.Background())
		}

		if err != nil {
			log.Println("Disable error:", err)
			return c.Send("Failed to disable reminders.")
		}
		return c.Send("Birthday Reminders are now disabled for this group.")
	})

	c := cron.New(cron.WithLocation(IST))

	_, err = c.AddFunc("5 0 * * *", func() {
		sendDailyReminders(b, db)
	})
	if err != nil {
		log.Fatal("Failed to schedule cron job:", err)
	}

	log.Println("Bot is starting and Database is connected...")
	c.Start()

	b.Start()
}

// We pass the *bun.DB here so the middleware can use it
func requireRemindersEnabled(db *bun.DB) func(tele.HandlerFunc) tele.HandlerFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			// always allow commands in DM
			if c.Chat().Type == tele.ChatPrivate {
				return next(c)
			}

			// always allow control commands so that users arent locked out
			if isControlCommand(c) {
				return next(c)
			}

			var enabled bool
			err := db.NewSelect().
				Table("group_settings").
				Column("enabled").
				Where("group_id = ?", c.Chat().ID).
				Scan(context.Background(), &enabled)

			// if no settings found, assume its enabled by default
			if err != nil {
				enabled = true
			}

			if !enabled {
				return c.Send("Birthday reminders are disabled in this group. \nUse /enable to turn then back on.")
			}

			return next(c)
		}
	}
}

func isControlCommand(c tele.Context) bool {
	text := strings.ToLower(c.Text())
	return strings.HasPrefix(text, "/enable") ||
		strings.HasPrefix(text, "/disable") ||
		strings.HasPrefix(text, "/help") ||
		strings.HasPrefix(text, "/start")
}

/*
func sendDailyReminders(b *tele.Bot, db *bun.DB) {
	now := time.Now().In(IST)
	day := now.Day()
	month := int(now.Month())

	log.Printf("Cron checking for birthdays at %s", now.Format("15:04:05"))
	var bdays []models.Birthday
	err := db.NewSelect().
		Model(&bdays).
		// This JOIN ensures we only get birthdays for groups that are NOT disabled
		Table("birthdays").
		//Join("LEFT JOIN group_settings AS gs ON gs.group_id = birthdays.group_id").
		Where("birthdays.day = ? AND birthdays.month = ?", day, month).
		//Where("gs.enabled IS NOT FALSE").
		Where("birthdays.group_id NOT IN (SELECT group_id FROM group_settings WHERE enabled = FALSE)").
		Scan(context.Background())

	if err != nil {
		log.Println("Cron birthday query error:", err)
		return
	}

	// group Names by their GroupID using a map
	groups := make(map[int64][]string)
	for _, bday := range bdays {
		groups[bday.GroupID] = append(groups[bday.GroupID], bday.Name)
	}

	// loop through map and send one message per grp
	for groupID, names := range groups {
		allNames := strings.Join(names, ", ")

		msg := fmt.Sprintf("Happy Birthday %s!!", allNames)

		_, err := b.Send(tele.ChatID(groupID), msg, tele.ModeMarkdown)
		if err != nil {
			log.Printf("failed to send message to group %d: %v", groupID, err)
		}
	}
}
*/

func sendDailyReminders(b *tele.Bot, db *bun.DB) {
	now := time.Now().In(IST)
	day := now.Day()
	month := int(now.Month())

	log.Printf("=== BIRTHDAY CRON STARTED at %s ===", now.Format("2006-01-02 15:04:05"))

	var bdays []models.Birthday
	err := db.NewSelect().
		Model(&bdays).
		Where("day = ? AND month = ?", day, month).
		Where("group_id NOT IN (SELECT group_id FROM group_settings WHERE enabled = FALSE)").
		Scan(context.Background())

	if err != nil {
		log.Printf("Query error: %v", err)
		return
	}

	if len(bdays) == 0 {
		log.Println("No birthdays today")
		return
	}

	log.Printf("Found %d birthday records today", len(bdays))

	// Group names (with deduplication just in case)
	groups := make(map[int64][]string)
	for _, b := range bdays {
		// Use map to deduplicate names per group (extra safety)
		nameSet := make(map[string]struct{})
		for _, existing := range groups[b.GroupID] {
			nameSet[existing] = struct{}{}
		}
		if _, exists := nameSet[b.Name]; !exists {
			groups[b.GroupID] = append(groups[b.GroupID], b.Name)
			nameSet[b.Name] = struct{}{}
		}
	}

	log.Printf("Sending to %d groups", len(groups))

	for groupID, names := range groups {
		allNames := strings.Join(names, ", ")
		msg := fmt.Sprintf("🎉 Happy Birthday %s! 🎂🥳", allNames)

		log.Printf("Sending to group %d: %s", groupID, msg)

		_, err := b.Send(tele.ChatID(groupID), msg)
		if err != nil {
			log.Printf("Failed to send to %d: %v", groupID, err)
		}
	}

	log.Println("=== BIRTHDAY CRON FINISHED ===")
}
