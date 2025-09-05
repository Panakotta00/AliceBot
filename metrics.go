package main

import (
	"bytes"
	"cmp"
	"encoding/gob"
	"image/color"
	"log"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"

	"github.com/bwmarrin/discordgo"
)

var Metrics = struct {
	LastStore time.Time
	Data      map[string]*[]int64
}{
	Data: make(map[string]*[]int64),
}

type IntValues []int64

func (v IntValues) Len() int            { return len(v) }
func (v IntValues) Value(i int) float64 { return float64(v[i]) + 0.01 }

func barChart(entries *[]int64, title string) (*plot.Plot, error) {
	plot.DefaultFont.Typeface = "gg sans"
	plot.DefaultFont.Style = font.StyleNormal
	plot.DefaultFont.Weight = font.WeightNormal
	plot.DefaultFont.Variant = ""
	plotter.DefaultFont = plot.DefaultFont
	plotter.DefaultLineStyle.Color = color.White

	p := plot.New()

	barsA, err := plotter.NewBarChart(IntValues(*entries), vg.Centimeter)
	if err != nil {
		return nil, err
	}
	barsA.LineStyle.Width = vg.Length(0)
	barsA.Color = plotutil.Color(0)
	p.Add(barsA)

	p.BackgroundColor = color.Transparent

	p.Title.Text = title
	p.Title.TextStyle.Color = color.White

	p.HideX()

	p.Y.Min = 0
	p.Y.Scale = SymlogScale{Base: 2, LinScale: 1, LinThresh: 20}
	//p.Y.Tick.Marker = SymlogTicks{Base: 10, LinThresh: 10}
	p.Y.Label.Text = "Msgs./Day"
	p.Y.Label.TextStyle.Color = color.White
	p.Y.Tick.Color = color.White
	p.Y.Tick.Label.Color = color.White
	p.Y.LineStyle.Color = color.White
	p.Y.AutoRescale = false
	if 1 > p.Y.Max {
		p.Y.Max = 1
	}

	return p, nil
}

type RewardPair struct {
	RoleID string
	Target int64
}

func init() {
	maps.Copy(commands, map[*discordgo.ApplicationCommand]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		{
			Name: "Alice Stats",
			Type: discordgo.UserApplicationCommand,
		}: func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			entries := Metrics.Data[i.Interaction.Member.User.ID]
			if entries == nil {
				entriesV := slices.Repeat([]int64{0}, Settings.NumTrackedDays)
				entries = &entriesV
			}

			historyPlot, err := barChart(entries, "Chat Stats History")

			img := vgimg.NewWith(vgimg.UseWH(16*vg.Centimeter, 9*vg.Centimeter), vgimg.UseBackgroundColor(color.RGBA{R: 20, G: 20, B: 24, A: 255}))
			inner := draw.Crop(draw.New(img), 1*vg.Centimeter, -1*vg.Centimeter, 1*vg.Centimeter, -1*vg.Centimeter)
			historyPlot.Draw(inner)

			var historyBuf bytes.Buffer
			png := vgimg.PngCanvas{Canvas: img}
			if _, err := png.WriteTo(&historyBuf); err != nil {
				log.Println(err)
				return
			}

			slices.Sort(*entries)
			sortedPlot, err := barChart(entries, "Chat Stats (Sorted)")

			roleTextStyle := historyPlot.Title.TextStyle
			roleTextStyle.Handler = plot.DefaultTextHandler
			minRight := vg.Length(0)
			rewards := []RewardPair{}
			for roleId, target := range Settings.RewardRole {
				role := roleCache[roleId]
				if role == nil {
					continue
				}
				rect := roleTextStyle.Rectangle(role.Name)
				if rect.Size().X > minRight {
					minRight = rect.Size().X
				}

				if float64(target)+1 > sortedPlot.Y.Max {
					sortedPlot.Y.Max = float64(target) + 1
				}

				rewards = append(rewards, RewardPair{role.ID, target})
			}
			slices.SortFunc(rewards, func(a, b RewardPair) int {
				return cmp.Compare(b.Target, a.Target)
			})

			img = vgimg.NewWith(vgimg.UseWH(16*vg.Centimeter, 9*vg.Centimeter), vgimg.UseBackgroundColor(color.RGBA{R: 20, G: 20, B: 24, A: 255}))
			inner = draw.Crop(draw.New(img), 1*vg.Centimeter, -1*vg.Centimeter-minRight, 1*vg.Centimeter, -1*vg.Centimeter)
			sortedPlot.Draw(inner)

			dataCanvas := sortedPlot.DataCanvas(inner)
			tx, ty := sortedPlot.Transforms(&dataCanvas)
			for _, reward := range rewards {
				roleId, target := reward.RoleID, reward.Target

				role := roleCache[roleId]
				if role == nil {
					continue
				}

				xs := tx(float64(Settings.NumTrackedDays-1)/2) - vg.Centimeter/2
				xe := tx(float64(Settings.NumTrackedDays))
				ys := ty(0)
				ye := ty(float64(target))

				style := sortedPlot.Y.LineStyle
				style.Width = 1 * vg.Millimeter
				style.Color = color.RGBA{uint8(role.Color >> 16), uint8(role.Color >> 8), uint8(role.Color), 0xFF}

				dataCanvas.StrokeLines(style, []vg.Point{{xs, ys}, {xs, ye}, {xe, ye}})

				textStyle := roleTextStyle
				textStyle.Color = style.Color
				dataCanvas.FillText(textStyle, vg.Point{xe, ye}, role.Name)
			}

			var sortedBuf bytes.Buffer
			png = vgimg.PngCanvas{Canvas: img}
			if _, err := png.WriteTo(&sortedBuf); err != nil {
				log.Println(err)
				return
			}

			flags := discordgo.MessageFlagsIsComponentsV2
			if i.Interaction.ChannelID != "769089313546960897" {
				flags = flags | discordgo.MessageFlagsEphemeral
			}

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Components: []discordgo.MessageComponent{
						discordgo.MediaGallery{
							Items: []discordgo.MediaGalleryItem{
								discordgo.MediaGalleryItem{
									Media: discordgo.UnfurledMediaItem{
										URL: "attachment://history.png",
									},
								},
							},
						},
						discordgo.MediaGallery{
							Items: []discordgo.MediaGalleryItem{
								discordgo.MediaGalleryItem{
									Media: discordgo.UnfurledMediaItem{
										URL: "attachment://sorted.png",
									},
								},
							},
						},
					},
					Flags: flags,
					Files: []*discordgo.File{
						&discordgo.File{
							Name:        "sorted.png",
							ContentType: "image/png",
							Reader:      bytes.NewReader(sortedBuf.Bytes()),
						},
						&discordgo.File{
							Name:        "history.png",
							ContentType: "image/png",
							Reader:      bytes.NewReader(historyBuf.Bytes()),
						},
					},
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

		s := strings.Builder{}
		for _, v := range entries {
			s.WriteString(strconv.FormatInt(v, 10))
			s.WriteString(" ")
		}
		//log.Println(user, s.String())

		slices.Sort(entries)
		var median int64
		if len(entries)%2 == 0 {
			median = (entries[len(entries)/2-1] + entries[len(entries)/2]) / 2
		} else {
			median = entries[len(entries)/2]
		}
		medians[user] = median
	}

	return medians
}
