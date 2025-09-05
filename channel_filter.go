package main

import (
	"log"

	"github.com/bwmarrin/discordgo"
	mapset "github.com/deckarep/golang-set/v2"
)

type ChannelFilter struct {
	IncludeCategories mapset.Set[string]
	IncludeChannels   mapset.Set[string]
	ExcludeChannels   mapset.Set[string]
	cache             mapset.Set[string]
}

type SerializedChannelFilter struct {
	IncludeCategories []string `toml:",multiline"`
	IncludeChannels   []string `toml:",multiline"`
	ExcludeChannels   []string `toml:",multiline"`
}

func (f *ChannelFilter) ToSerialized() *SerializedChannelFilter {
	return &SerializedChannelFilter{
		IncludeCategories: f.IncludeCategories.ToSlice(),
		IncludeChannels:   f.IncludeChannels.ToSlice(),
		ExcludeChannels:   f.ExcludeChannels.ToSlice(),
	}
}

func (f *SerializedChannelFilter) ToUnserialized() ChannelFilter {
	var filter = ChannelFilter{
		IncludeCategories: mapset.NewSet[string](),
		IncludeChannels:   mapset.NewSet[string](),
		ExcludeChannels:   mapset.NewSet[string](),
	}
	if f != nil {
		if f.IncludeCategories != nil {
			filter.IncludeCategories = mapset.NewSet[string](f.IncludeCategories...)
		}
		if f.IncludeChannels != nil {
			filter.IncludeChannels = mapset.NewSet[string](f.IncludeChannels...)
		}
		if f.ExcludeChannels != nil {
			filter.ExcludeChannels = mapset.NewSet[string](f.ExcludeChannels...)
		}
	}

	return filter
}

func (f *ChannelFilter) matchID(channel string) bool {
	if f.ExcludeChannels.Contains(channel) {
		return false
	}

	if f.IncludeChannels.Contains(channel) {
		return true
	}

	return false
}

func (f *ChannelFilter) matchChannel(channel *discordgo.Channel) bool {
	if f.ExcludeChannels.Contains(channel.ID) {
		return false
	}

	if f.matchID(channel.ID) {
		return true
	}

	if f.IncludeCategories.Contains(channel.ParentID) {
		return true
	}

	return false
}

func (f *ChannelFilter) matchCachedID(channel string) bool {
	if f.cache != nil {
		return f.cache.Contains(channel)
	} else {
		return f.matchID(channel)
	}
}

func (f *ChannelFilter) matchCachedChannel(channel *discordgo.Channel) bool {
	if f.cache != nil {
		return f.cache.Contains(channel.ID)
	} else {
		return f.matchChannel(channel)
	}
}

func (f *ChannelFilter) updateCache(dg *discordgo.Session) {
	channels, err := dg.GuildChannels(guild)
	if err != nil {
		log.Panicln("Error getting guild channels: %v", err)
	}

	f.cache = mapset.NewSet[string]()
	for _, channel := range channels {
		if f.matchChannel(channel) {
			f.cache.Add(channel.ID)
		}
	}
}
