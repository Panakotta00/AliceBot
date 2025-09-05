package main

import (
	"bytes"
	"encoding/gob"
	"log"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var Metrics = struct {
	LastStore time.Time
	Data      map[string]*[]int64
}{
	Data: make(map[string]*[]int64),
}

func init() {
	maps.Copy(commands, map[*discordgo.ApplicationCommand]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		{
			Name: "Alice Metrics",
			Type: discordgo.UserApplicationCommand,
		}: func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "meep",
				},
			})
			if err != nil {
				log.Println(err)
			}
		},
	})

	loadMetrics()
}

func addPoints(user string, points int64) {
	entry, ok := Metrics.Data[user]
	if !ok {
		entries := make([]int64, Settings.NumTrackedDays)
		entry = &entries
		Metrics.Data[user] = entry
	}
	(*entry)[0] += points
}

func metricMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if !Settings.metricChannelFilter.matchCachedID(m.ChannelID) {
		return
	}
	if s.State.User.ID == m.Author.ID {
		return
	}

	addPoints(m.Author.ID, 1)
}

func stepCumulation() {
	var toRemove []string
	for user, entry := range Metrics.Data {
		*entry = slices.Insert(*entry, 0, 0)
		*entry = slices.Delete(*entry, Settings.NumTrackedDays, len(*entry))

		var sum int64 = 0
		for _, v := range *entry {
			sum += v
		}
		if sum == 0 {
			toRemove = append(toRemove, user)
		}
	}

	for _, user := range toRemove {
		delete(Metrics.Data, user)
	}
}

func loadMetrics() {
	b, err := os.ReadFile("metrics.gob")
	if err == nil {
		b := bytes.NewBuffer(b)
		dec := gob.NewDecoder(b)
		if err = dec.Decode(&Metrics); err != nil {
			log.Panicf("Failed to load metrics: %e", err)
		}

		for _, entries := range Metrics.Data {
			insertionPoint := 0          // change insertion point based on metrics last stored
			var insertionValue int64 = 0 // change insertion value based on already existing metrics
			for len(*entries) < Settings.NumTrackedDays {
				*entries = slices.Insert(*entries, insertionPoint, insertionValue)
			}
			*entries = slices.Delete(*entries, Settings.NumTrackedDays, len(*entries))
		}
	}
}

func storeMetrics() {
	Metrics.LastStore = time.Now()

	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	if err := enc.Encode(Metrics); err != nil {
		log.Println("Failed to save metrics! %e", err)
	}

	err := os.WriteFile("metrics.gob", b.Bytes(), 0644)
	if err != nil {
		log.Panicln("Failed to save metrics! %e", err)
	}
	log.Println("Metrics saved.")
}

func analyzeMetrics() map[string]int64 {
	var medians = map[string]int64{}

	for user, entries := range Metrics.Data {
		entries := slices.Clone(*entries)
		slices.Sort(entries)
		var median int64
		if len(entries)%2 == 0 {
			median = (entries[len(entries)/2-1] + entries[len(entries)/2]) / 2
		} else {
			median = entries[len(entries)/2]
		}
		medians[user] = median

		s := strings.Builder{}
		for _, v := range entries {
			s.WriteString(strconv.FormatInt(v, 10))
			s.WriteString(" ")
		}

		log.Println(user, median, s.String())
	}

	return medians
}
