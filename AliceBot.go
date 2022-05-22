package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	gocache "github.com/patrickmn/go-cache"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const QUANTIZATION_PERIOD = 24 * time.Hour
const EVALUATION_PERIOD = 7 * 24 * time.Hour
const ROLE_UPDATE_PERIOD = 10 * time.Minute
const THRESHOLD = 20
const CACHE_PERSISTENCE_PERIOD = 1 * time.Minute

const GUILD = "876229697334296617"
const ROLE = "977692417576816640"

//const GUILD = "769084944373776415"
//const ROLE = "977728199523983370"

var MessageFilter Filter = &CategoryFilter{GuildId: GUILD, CategoryIDs: []string{"978049788383752312"}}

var ctx context.Context

func checkErr(err error) {
	if err != nil {
		log.Fatalf("Error: %+v\n", err)
	}
}

func ctxWithCache(ctx context.Context, cache *gocache.Cache) context.Context {
	return context.WithValue(ctx, "cache", cache)
}

func cacheFromCtx(ctx context.Context) *gocache.Cache {
	return ctx.Value("cache").(*gocache.Cache)
}

func ctxWithDB(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, "db", db)
}

func dbFromCtx(ctx context.Context) *sql.DB {
	return ctx.Value("db").(*sql.DB)
}

func ctxWithDiscord(ctx context.Context, dg *discordgo.Session) context.Context {
	return context.WithValue(ctx, "discord", dg)
}

func discordFromCtx(ctx context.Context) *discordgo.Session {
	return ctx.Value("discord").(*discordgo.Session)
}

func main() {
	log.Println("Start AliceBot...")

	log.Println("Create Database Connection...")
	db, err := sql.Open("sqlite3", os.Getenv("DB_PATH"))
	checkErr(err)
	log.Println("Database Connection Established.")

	log.Println("Create Discord Connection...")
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	checkErr(err)
	dg.AddHandler(messageCreate)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		HandleCommand(ctx, s, i)
	})
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	checkErr(err)
	MessageFilter.InitFilter(dg)
	log.Println("Discord Connection Established...")

	log.Println("Clean-up existing commands...")
	cmds, _ := dg.ApplicationCommands(dg.State.User.ID, GUILD)
	if cmds != nil {
		for _, cmd := range cmds {
			_ = dg.ApplicationCommandDelete(dg.State.User.ID, GUILD, cmd.ID)
		}
	}
	log.Println("Existing Commands Cleaned-Up.")

	log.Println("Add Commands...")
	for _, cmd := range Commands {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, GUILD, cmd.Cmd)
		if err != nil {
			log.Printf("Unable to create command '%s': %+v\n", cmd.Cmd.Name, err)
		}
	}
	log.Println("Commands added.")

	log.Println("Create Cache...")
	cache := gocache.New(5*time.Minute, 10*time.Minute)
	log.Println("Cache created.")

	log.Println("Unpersist existing Cache...")
	unpersistCache(cache)
	log.Println("Existing Cache Unpersisted.")

	ctx = context.Background()
	ctx = ctxWithCache(ctx, cache)
	ctx = ctxWithDB(ctx, db)
	ctx = ctxWithDiscord(ctx, dg)

	log.Println("Create Flush Ticker...")
	flushTicker := time.NewTicker(QUANTIZATION_PERIOD)
	killFlushTicker := make(chan bool)
	defer func() { killFlushTicker <- true }()
	go func() {
		for {
			select {
			case <-killFlushTicker:
				return
			case <-flushTicker.C:
				flushUsersToDB(ctx)
			}
		}
	}()
	log.Println("Flush Tickers created.")

	log.Println("Create Role Update Ticker...")
	roleUpdateTicker := time.NewTicker(ROLE_UPDATE_PERIOD)
	killRoleUpdateTicker := make(chan bool)
	defer func() { killRoleUpdateTicker <- true }()
	go func() {
		for {
			select {
			case <-killRoleUpdateTicker:
				return
			case <-roleUpdateTicker.C:
				updateUserRoles(ctx)
			}
		}
	}()
	log.Println("Role Update Ticker created.")

	log.Println("Create Cache Persistence Ticker...")
	cachePersistenceTicker := time.NewTicker(CACHE_PERSISTENCE_PERIOD)
	killCachePersistenceTicker := make(chan bool)
	defer func() { killCachePersistenceTicker <- true }()
	go func() {
		for {
			select {
			case <-killCachePersistenceTicker:
				return
			case <-cachePersistenceTicker.C:
				persistCache(ctx)
			}
		}
	}()
	log.Println("Cache Persistence Ticker created.")

	updateUserRoles(ctx)

	log.Println("AliceBot started!")

	// Wait here until CTRL-C or other term signal is received.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	_ = dg.Close()
	_ = db.Close()
}

func flushUsersToDB(ctx context.Context) {
	cache := cacheFromCtx(ctx)
	db := dbFromCtx(ctx)

	items := cache.Items()

	if len(items) < 1 {
		return
	}

	log.Println("Flush Users to DB...")
	insertArgs := make([]string, len(items))
	insertValues := make([]interface{}, len(items)*2)
	i := 0
	for user, count := range items {
		insertArgs[i] = "(?,?)"
		insertValues[i*2] = user
		insertValues[i*2+1] = count.Object
		i++
	}
	_, err := db.Exec(fmt.Sprintf(`
		INSERT INTO user_messages
			(user_id, message_count)
		VALUES %s
	`, strings.Join(insertArgs, ",")), insertValues...)
	cache.Flush()
	if err != nil {
		log.Printf("Error on Flush: %+v\n", err)
	} else {
		log.Println("Users flushed to DB!")
	}
}

func updateUserRoles(ctx context.Context) {
	db := dbFromCtx(ctx)
	dg := discordFromCtx(ctx)

	log.Println("Update User Roles...")

	log.Println("Fetch previous active Users...")
	rows, err := db.Query(`
		SELECT user_id
		FROM active_user
	`)
	if err != nil {
		log.Printf("Error on User Role Update - Fetch Active Users: %+v\n", err)
		return
	}
	activeUsers := map[string]bool{}
	for rows.Next() {
		var user_id string
		err := rows.Scan(&user_id)
		if err == nil {
			activeUsers[user_id] = true
		}
	}

	log.Println("Fetch evaluation period...")
	rows, err = db.Query(`
		SELECT user_id, message_count
	   	FROM user_messages
	   	WHERE timestamp >= datetime('now', ? || ' seconds')
	   	ORDER BY user_id, message_count DESC
	`, -EVALUATION_PERIOD.Seconds())
	if err != nil {
		log.Printf("Error on User Role Update: %+v\n", err)
		return
	}

	quantaizationCount := int(math.Round(EVALUATION_PERIOD.Seconds() / QUANTIZATION_PERIOD.Seconds()))

	log.Println("Evaluate Users...")
	data := make(map[string][]int64)
	dataCount := make(map[string]int64)
	for rows.Next() {
		var id string
		var count int64
		if rows.Scan(&id, &count) == nil {
			if data[id] == nil {
				data[id] = make([]int64, quantaizationCount)
				dataCount[id] = 0
			} else {
				dataCount[id] = dataCount[id] + 1
			}
			data[id][dataCount[id]] = count
		}
	}

	log.Println("Add roles to now active users...")
	var usersToAdd []interface{}
	for user, counts := range data {
		score := counts[quantaizationCount/2]
		if score > THRESHOLD {
			if !activeUsers[user] {
				err := dg.GuildMemberRoleAdd(GUILD, user, ROLE)
				if err == nil {
					usersToAdd = append(usersToAdd, user)
					log.Println("Now Active:", user)
				}
			}
			activeUsers[user] = false
		}
	}
	if len(usersToAdd) > 0 {
		log.Println("Add now active users to DB...")
		_, err = db.Exec(fmt.Sprintf(`
			INSERT INTO active_user
				(user_id)
			VALUES (?%s)
		`, strings.Repeat(",?", len(usersToAdd)-1)), usersToAdd...)
		if err != nil {
			log.Printf("Error on User Role Update - Add Active Users: %+v\n", err)
			return
		}
	}

	log.Println("Remove roles from now inactive users...")
	var usersToDelete []interface{}
	for user, active := range activeUsers {
		if active {
			err := dg.GuildMemberRoleRemove(GUILD, user, ROLE)
			if err != nil {
				activeUsers[user] = false
			} else {
				usersToDelete = append(usersToDelete, user)
				log.Println("Now Inactive:", user)
			}
		}
	}

	if len(usersToDelete) > 0 {
		log.Println("Remove now inactive users from DB...")
		_, err = db.Exec(fmt.Sprintf(`
			DELETE FROM active_user
			WHERE user_id IN (?%s)
		`, strings.Repeat(",?", len(usersToDelete)-1)), usersToDelete...)
		if err != nil {
			log.Printf("Error on User Role Update - Remove Inactive Users: %+v\n", err)
			return
		}
	}

	log.Println("User Roles Updated!")
}

func unpersistCache(cache *gocache.Cache) {
	data, err := ioutil.ReadFile(os.Getenv("CACHE_PATH"))
	if err != nil {
		return
	}

	var entries map[string]int64
	err = json.Unmarshal(data, &entries)
	if err != nil {
		return
	}

	for userId, entry := range entries {
		_ = cache.Add(userId, entry, gocache.NoExpiration)
	}
}

func persistCache(ctx context.Context) {
	cache := cacheFromCtx(ctx)

	items := cache.Items()
	var entries map[string]int64 = make(map[string]int64, len(items))
	for userId, item := range items {
		entries[userId] = item.Object.(int64)
	}

	data, err := json.MarshalIndent(entries, "", "\t")
	if err != nil {
		return
	}

	_ = ioutil.WriteFile(os.Getenv("CACHE_PATH"), data, 0755)
}

func incrementUser(ctx context.Context, userId string) {
	cache := cacheFromCtx(ctx)
	count, err := cache.IncrementInt64(userId, 1)
	if err != nil {
		_ = cache.Add(userId, int64(1), gocache.NoExpiration)
		count = 1
	}

	log.Printf("Count '%s': %d\n", userId, count)
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	if !MessageFilter.FilterMessage(m.Message) {
		return
	}
	incrementUser(ctx, m.Author.ID)
}

func memberBatch(s *discordgo.Session, m *discordgo.GuildMembersChunk) {

}
