package main

import (
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Cmd *discordgo.ApplicationCommand

	Filter Filter

	OnExec func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate)
}

func ActivityScoreInteraction(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	cache := cacheFromCtx(ctx)

	var requestedUser *discordgo.User
	switch t := i.Data.(type) {
	case discordgo.ApplicationCommandInteractionData:
		if len(t.TargetID) > 0 {
			requestedUser = t.Resolved.Users[t.TargetID]
		} else {
			requestedUser = i.Member.User
		}
	default:
		requestedUser = i.Member.User
	}

	score, _ := cache.Get(requestedUser.ID)

	var response string
	if requestedUser.ID == i.Member.User.ID {
		response = "Your"
	} else {
		response = fmt.Sprintf("<@%s>'s", requestedUser.ID)
	}
	if score == nil {
		response = response + " activity score in this this quantization period is 0."
	} else {
		response = response + fmt.Sprintf(" activity score in this quantization period is %d.", score)
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response,
		},
	})
}

var (
	Commands = []*Command{
		{
			Cmd: &discordgo.ApplicationCommand{
				Name:        "activity_score",
				Description: "Returns your personal activity score of the current quantization period.",
			},
			OnExec: func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
				ActivityScoreInteraction(ctx, s, i)
			},
		}, {
			Cmd: &discordgo.ApplicationCommand{
				Name: "Activity Score",
				Type: discordgo.UserApplicationCommand,
			},
			OnExec: func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
				ActivityScoreInteraction(ctx, s, i)
			},
		},
	}
)

func HandleCommand(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	name := i.ApplicationCommandData().Name
	for _, cmd := range Commands {
		if cmd.Cmd.Name == name {
			if cmd.Filter == nil || cmd.Filter.FilterMessage(i.Message) {
				cmd.OnExec(ctx, s, i)
			}
			return
		}
	}
}
