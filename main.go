package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron"
)

type Metric struct {
	Time         time.Time
	MessageCount int64
}

type Store struct {
	Metrics []Metric
}

var token string
var app string
var guild string

var dg *discordgo.Session
var cronJobs *cron.Cron

var commandCache = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}

func i64(v int64) *int64 { return &v }

func init() {
	flag.StringVar(&token, "t", os.Getenv("DISCORD_TOKEN"), "Bot Token")
	flag.StringVar(&app, "a", os.Getenv("DISCORD_APP"), "Application ID")
	flag.StringVar(&guild, "g", os.Getenv("DISCORD_GUILD"), "Guild ID")
	flag.Parse()
}

func updateAllowedChannels(dg *discordgo.Session) {
	Settings.metricChannelFilter.updateCache(dg)
}

func reloadCron() {
	if cronJobs != nil {
		cronJobs.Stop()
	}

	cronJobs := cron.New()
	err := cronJobs.AddFunc(Settings.Cron.CumulationStep, stepCumulation)
	if err != nil {
		log.Println(err)
	}
	err = cronJobs.AddFunc(Settings.Cron.SaveMetrics, storeMetrics)
	if err != nil {
		log.Println(err)
	}
	err = cronJobs.AddFunc(Settings.Cron.UpdateRewards, updateRewards)
	if err != nil {
		log.Println(err)
	}
	cronJobs.Start()
}

func main() {
	for cmd, f := range commands {
		commandCache[cmd.Name] = f
	}

	log.Println("Starting Bot...")
	var err error
	dg, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Panicln("error creating Discord session,", err)
	}

	dg.AddHandler(metricMessage)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			f, ok := commandCache[i.ApplicationCommandData().Name]
			if ok {
				f(s, i)
			}
			break
		case discordgo.InteractionMessageComponent:
			f, ok := messageComponents[i.MessageComponentData().CustomID]
			if ok {
				f(s, i)
			}
			break
		case discordgo.InteractionModalSubmit:
			ids := strings.Split(i.ModalSubmitData().CustomID, "|")
			f, ok := modals[ids[0]]
			if ok {
				f(s, i, ids)
			}
			break
		}
	})
	dg.AddHandler(func(s *discordgo.Session, c *discordgo.ChannelCreate) {
		updateAllowedChannels(s)
	})
	dg.AddHandler(func(s *discordgo.Session, c *discordgo.ChannelUpdate) {
		updateAllowedChannels(s)
	})
	dg.AddHandler(func(s *discordgo.Session, c *discordgo.ChannelDelete) {
		updateAllowedChannels(s)
	})

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildIntegrations | discordgo.IntentsGuildMembers

	_, err = dg.ApplicationCommandBulkOverwrite(app, guild, slices.Collect(maps.Keys(commands)))
	if err != nil {
		log.Panicln("could not register commands: %s", err)
	}

	err = dg.Open()
	if err != nil {
		log.Panicln("error opening connection,", err)
	}

	updateAllowedChannels(dg)

	reloadCron()

	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Shutting down...")

	dg.Close()
	log.Println("Bot stopped!")
}

func test() {
	store := Store{Metrics: make([]Metric, 0)}
	t, _ := time.Parse("2006-01-02 15:04:05", "2006-01-02 15:04:05")
	store.Metrics = append(store.Metrics, Metric{
		Time:         t,
		MessageCount: 1,
	})

	store.Metrics = slices.Collect(func(yield func(Metric) bool) {
		for _, metric := range store.Metrics {
			if time.Since(metric.Time) > time.Hour*24*7 {
				continue
			}
			if !yield(metric) {
				return
			}
		}
	})

	var b bytes.Buffer

	enc := gob.NewEncoder(&b)
	if err := enc.Encode(store); err != nil {
		fmt.Println("Error encoding struct:", err)
		return
	}

	serializedData := b.Bytes()
	fmt.Println("Serialized data:", serializedData)
}
