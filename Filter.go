package main

import (
	"github.com/bwmarrin/discordgo"
	"golang.org/x/exp/slices"
)

type Filter interface {
	FilterMessage(*discordgo.Message) bool
	InitFilter(*discordgo.Session)
}

type ChannelFilter struct {
	ChannelIDs []string
}

func (c *ChannelFilter) InitFilter(*discordgo.Session) {}

func (c *ChannelFilter) FilterMessage(msg *discordgo.Message) bool {
	_, found := slices.BinarySearch(c.ChannelIDs, msg.ChannelID)
	return found
}

type CategoryFilter struct {
	channelFilter ChannelFilter

	GuildId     string
	CategoryIDs []string
}

func (c *CategoryFilter) InitFilter(s *discordgo.Session) {
	channels, err := s.GuildChannels(c.GuildId)
	if err != nil {
		return
	}

	for _, channel := range channels {
		_, found := slices.BinarySearch(c.CategoryIDs, channel.ParentID)
		if found {
			c.channelFilter.ChannelIDs = append(c.channelFilter.ChannelIDs, channel.ID)
		}
	}
}

func (c *CategoryFilter) FilterMessage(msg *discordgo.Message) bool {
	return c.channelFilter.FilterMessage(msg)
}
