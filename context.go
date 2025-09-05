package main

import (
	"github.com/bwmarrin/discordgo"
)

var messageComponents = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
var commands = map[*discordgo.ApplicationCommand]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
var modals = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate, ids []string){}
