package main

import (
	"cmp"
	"log"
	"slices"
)

type pair struct {
	User string
	Val  int64
}

func updateRewards() {
	medians := analyzeMetrics()

	var sortedMedians []pair
	for user, val := range medians {
		sortedMedians = append(sortedMedians, pair{user, val})
	}

	slices.SortFunc(sortedMedians, func(i pair, j pair) int {
		return cmp.Compare(j.Val, i.Val)
	})

	targetRoles := map[string]*[]string{
		Settings.KingsRole: {},
	}
	for role := range Settings.RewardRole {
		targetRoles[role] = &[]string{}
	}

	for i, entry := range sortedMedians {
		if entry.Val < 1 {
			continue
		}

		if i < 6 {
			kings := targetRoles[Settings.KingsRole]
			*kings = append(*kings, entry.User)
		}

		for role, target := range Settings.RewardRole {
			if entry.Val >= target {
				targetRole := targetRoles[role]
				*targetRole = append(*targetRole, entry.User)
			}
		}
	}

	after := ""
	for {
		batch, err := dg.GuildMembers(guild, after, 1000)
		if err != nil {
			log.Printf("Failed to get guild members: %e", err)
			break
		}
		if len(batch) == 0 {
			break
		}
		after = batch[len(batch)-1].User.ID

		for _, member := range batch {
			for role, users := range targetRoles {
				if role == "" {
					continue
				}

				shouldHaveRole := slices.Contains(*users, member.User.ID)
				hasRole := slices.Contains(member.Roles, role)

				if shouldHaveRole && !hasRole {
					err := dg.GuildMemberRoleAdd(guild, member.User.ID, role)
					if err != nil {
						log.Printf("Failed to add role %s to %s: %e", role, member.User.ID, err)
					}
				} else if !shouldHaveRole && hasRole {
					err := dg.GuildMemberRoleRemove(guild, member.User.ID, role)
					if err != nil {
						log.Printf("Failed to remove role %s from %s: %e", role, member.User.ID, err)
					}
				}
			}
		}

		if len(batch) < 1000 {
			break
		}
	}
}
