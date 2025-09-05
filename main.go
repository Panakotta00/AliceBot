package main

import (
	"bytes"
	"flag"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron"
	goFont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/font"

	_ "time/tzdata"

	_ "golang.org/x/crypto/x509roots/fallback"
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
var roleCache = map[string]*discordgo.Role{}

func i64(v int64) *int64 { return &v }

func init() {
	flag.StringVar(&token, "t", os.Getenv("DISCORD_TOKEN"), "Bot Token")
	flag.StringVar(&app, "a", os.Getenv("DISCORD_APP"), "Application ID")
	flag.StringVar(&guild, "g", os.Getenv("DISCORD_GUILD"), "Guild ID")
	flag.Parse()

	loadDiscordFontCache()
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

func updateRoleCache() {
	roleCache = map[string]*discordgo.Role{}
	roles, err := dg.GuildRoles(guild)
	if err != nil {
		log.Println(err)
		return
	}
	for _, role := range roles {
		roleCache[role.ID] = role
	}
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
	updateRoleCache()

	reloadCron()

	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Shutting down...")

	dg.Close()
	log.Println("Bot stopped!")
}

func loadDiscordFontCache() {
	paths := []struct {
		path   string
		style  goFont.Style
		weight goFont.Weight
	}{
		{"https://fonts.cdnfonts.com/s/93931/gg sans Regular.ttf", goFont.StyleNormal, goFont.WeightNormal},
	}

	col := font.Collection{}
	for _, it := range paths {
		resp, err := http.Get(it.path)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		data := bytes.NewBuffer(nil)
		if _, err := data.ReadFrom(resp.Body); err != nil {
			continue
		}
		sf, err := sfnt.Parse(data.Bytes())
		if err != nil {
			continue
		}
		col = append(col, font.Face{
			Face: sf,
			Font: font.Font{
				Typeface: "gg sans",
				Style:    it.style,
				Weight:   it.weight,
			},
		})
	}

	plot.DefaultTextHandler.Cache().Add(col)
}
