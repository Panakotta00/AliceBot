package main

import (
	"fmt"
	"log"
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pelletier/go-toml"
)

type CronSettings struct {
	SaveMetrics    string
	UpdateRewards  string
	CumulationStep string
}

var Settings = struct {
	NumTrackedDays      int
	metricChannelFilter ChannelFilter
	KingsRole           string
	RewardRole          map[string]int64

	Cron CronSettings

	MetricChannelFilterSerialized *SerializedChannelFilter `toml:"MetricChannels"`
}{
	NumTrackedDays: 7,
	metricChannelFilter: ChannelFilter{
		IncludeCategories: mapset.NewSet[string](),
		IncludeChannels:   mapset.NewSet[string](),
		ExcludeChannels:   mapset.NewSet[string](),
	},
	KingsRole:  "",
	RewardRole: map[string]int64{},
	Cron: CronSettings{
		SaveMetrics:    "*/5 * * * *",
		UpdateRewards:  "*/5 * * * *",
		CumulationStep: "*/5 * * * *",
	},
	MetricChannelFilterSerialized: nil,
}

func SeparatorSpacingSizePtr(s discordgo.SeparatorSpacingSize) *discordgo.SeparatorSpacingSize {
	return &s
}

func createSettings(s *discordgo.Session) []discordgo.MessageComponent {
	var msg strings.Builder
	msg.WriteString("# Metrics\n")
	msg.WriteString("**Included Categories:**\n")
	for c := range Settings.metricChannelFilter.IncludeCategories.Iter() {
		msg.WriteString(fmt.Sprintf("* <#%s>\n", c))
	}
	msg.WriteString("**Included Channels:**\n")
	for c := range Settings.metricChannelFilter.IncludeChannels.Iter() {
		msg.WriteString(fmt.Sprintf("* <#%s>\n", c))
	}
	msg.WriteString("**Excluded Channels:**\n")
	for c := range Settings.metricChannelFilter.ExcludeChannels.Iter() {
		msg.WriteString(fmt.Sprintf("* <#%s>\n", c))
	}
	msg.WriteString("# Rewards\n")
	msg.WriteString(fmt.Sprintf("Top 6: <@&%s>\n", Settings.KingsRole))
	for role, target := range Settings.RewardRole {
		msg.WriteString(fmt.Sprintf("* <@&%s> (%d)\n", role, target))
	}

	kingsDefault := []discordgo.SelectMenuDefaultValue{}
	if Settings.KingsRole != "" {
		kingsDefault = []discordgo.SelectMenuDefaultValue{{
			Type: discordgo.SelectMenuDefaultValueRole,
			ID:   Settings.KingsRole,
		}}
	}

	return []discordgo.MessageComponent{
		discordgo.TextDisplay{
			Content: msg.String(),
		},
		discordgo.Separator{
			Spacing: SeparatorSpacingSizePtr(discordgo.SeparatorSpacingSizeLarge),
		},
		discordgo.TextDisplay{
			Content: "Toggle Include Channel/Category:",
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.ChannelSelectMenu,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildCategory,
						discordgo.ChannelTypeGuildText,
					},
					CustomID: "toggle_include",
				},
			},
		},
		discordgo.TextDisplay{
			Content: "Toggle Exclude Channel:",
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.ChannelSelectMenu,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
					CustomID: "toggle_exclude",
				},
			},
		},
		discordgo.TextDisplay{
			Content: "Change Top 5 Role:",
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:      discordgo.RoleSelectMenu,
					DefaultValues: kingsDefault,
					CustomID:      "change_kings_role",
				},
			},
		},
		discordgo.TextDisplay{
			Content: "Add new Role Reward:",
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.RoleSelectMenu,
					CustomID: "add_reward",
				},
			},
		},
		discordgo.TextDisplay{
			Content: "Remove Role Reward:",
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.RoleSelectMenu,
					CustomID: "remove_reward",
				},
			},
		},
	}
}

func saveSettings() {
	Settings.MetricChannelFilterSerialized = Settings.metricChannelFilter.ToSerialized()

	log.Println("Saving settings...")
	b, err := toml.Marshal(&Settings)
	if err != nil {
		log.Panicln(err)
	}

	err = os.WriteFile("settings.toml", b, 0644)
	if err != nil {
		log.Panicf("unable to save settings: %e", err)
	}

	Settings.MetricChannelFilterSerialized = nil

	log.Println("Settings saved.")
}

func loadSettings() {
	log.Println("Loading settings...")
	if _, err := os.Stat("settings.toml"); err == nil {
		b, err := os.ReadFile("settings.toml")
		if err != nil {
			log.Panicf("unable to load settings: %e", err)
		}

		err = toml.Unmarshal(b, &Settings)
		if err != nil {
			log.Panicln(err)
		}

		Settings.metricChannelFilter = Settings.MetricChannelFilterSerialized.ToUnserialized()
		Settings.MetricChannelFilterSerialized = nil

		log.Println("Settings loaded.")
	} else {
		log.Println("No settings file found. Load and save default settings.")
		saveSettings()
	}
}

func init() {
	loadSettings()

	maps.Copy(commands, map[*discordgo.ApplicationCommand]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		{
			Name:                     "alice_settings",
			Description:              "Prints the current alice bot configuration.",
			Type:                     discordgo.ChatApplicationCommand,
			DefaultMemberPermissions: i64(0),
		}: func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Components:      createSettings(s),
					AllowedMentions: &discordgo.MessageAllowedMentions{},
					Flags:           discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println(err)
			}
		},
	})

	maps.Copy(messageComponents, map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"toggle_include": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			for _, c := range i.MessageComponentData().Values {
				channel, err := s.Channel(c)
				if err != nil {
					continue
				}
				switch channel.Type {
				case discordgo.ChannelTypeGuildCategory:
					if Settings.metricChannelFilter.IncludeCategories.Contains(channel.ID) {
						Settings.metricChannelFilter.IncludeCategories.Remove(channel.ID)
					} else {
						Settings.metricChannelFilter.IncludeCategories.Add(channel.ID)
					}
				case discordgo.ChannelTypeGuildText:
					if Settings.metricChannelFilter.IncludeChannels.Contains(channel.ID) {
						Settings.metricChannelFilter.IncludeChannels.Remove(channel.ID)
					} else {
						Settings.metricChannelFilter.IncludeChannels.Add(channel.ID)
					}
				}
			}

			saveSettings()

			updateSettingsMessage(s, i)

			updateAllowedChannels(s)
		},
		"toggle_exclude": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			for _, c := range i.MessageComponentData().Values {
				channel, err := s.Channel(c)
				if err != nil {
					continue
				}
				switch channel.Type {
				case discordgo.ChannelTypeGuildText:
					if Settings.metricChannelFilter.ExcludeChannels.Contains(channel.ID) {
						Settings.metricChannelFilter.ExcludeChannels.Remove(channel.ID)
					} else {
						Settings.metricChannelFilter.ExcludeChannels.Add(channel.ID)
					}
				}
			}

			saveSettings()

			updateSettingsMessage(s, i)

			updateAllowedChannels(s)
		},
		"change_kings_role": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			Settings.KingsRole = i.MessageComponentData().Values[0]
			saveSettings()

			updateSettingsMessage(s, i)
		},
		"add_reward": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					Title: "Add New Role Reward",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.TextInput{
									Label:       "Target",
									Placeholder: "Target to reach",
									Style:       discordgo.TextInputShort,
									Required:    true,
									CustomID:    "reward_target",
								},
							},
						},
					},
					CustomID: fmt.Sprintf("add_reward|%s", i.MessageComponentData().Values[0]),
					Flags:    discordgo.MessageFlagsIsComponentsV2,
				},
			})
			if err != nil {
				log.Println(err)
			}
		},
		"remove_reward": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			delete(Settings.RewardRole, i.MessageComponentData().Values[0])
			saveSettings()

			updateSettingsMessage(s, i)
		},
	})

	maps.Copy(modals, map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate, ids []string){
		"add_reward": func(s *discordgo.Session, i *discordgo.InteractionCreate, ids []string) {
			var targetStr = i.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
			target, err := strconv.ParseInt(targetStr, 10, 64)
			if err != nil {
				return
			}

			Settings.RewardRole[ids[1]] = target

			updateSettingsMessage(s, i)
		},
	})
}

func updateSettingsMessage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	msg := createSettings(s)
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Components:      msg,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
			Flags:           discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println(err)
	}
}
