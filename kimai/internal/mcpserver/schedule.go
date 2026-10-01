package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type WorkBlock struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"`
}

type Schedule struct {
	Timezone string                 `json:"timezone"`
	Days     map[string][]WorkBlock `json:"days"`
}

var weekdayNames = []string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

func LoadSchedule(raw string) *Schedule {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var s Schedule
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return buildDefaultSchedule()
		}
		if s.Days == nil {
			s.Days = map[string][]WorkBlock{}
		}
		if s.Timezone == "" {
			s.Timezone = "America/Sao_Paulo"
		}
		return &s
	}
	return buildDefaultSchedule()
}

func buildDefaultSchedule() *Schedule {
	blocks := []WorkBlock{
		{Start: "09:00", End: "09:30", Label: "Daily"},
		{Start: "09:30", End: "12:00", Label: "Manhã"},
		{Start: "13:00", End: "18:00", Label: "Tarde"},
	}
	s := &Schedule{Timezone: "America/Sao_Paulo", Days: map[string][]WorkBlock{}}
	for _, d := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday"} {
		s.Days[d] = blocks
	}
	return s
}

func (s *Schedule) BlocksFor(t time.Time) []WorkBlock {
	if s == nil {
		return nil
	}
	if blocks, ok := s.Days[t.Weekday().String()]; ok {
		return blocks
	}
	if blocks, ok := s.Days["Default"]; ok {
		return blocks
	}
	return nil
}

func (s *Schedule) location() *time.Location {
	if s == nil || s.Timezone == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	return time.Local
}

func blockTime(day time.Time, hhmm string) (time.Time, bool) {
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return time.Time{}, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location()), true
}

func formatScheduleSection(s *Schedule) string {
	if s == nil {
		return "🗓️ **Horário fixo:** nenhum configurado (defina KIMAI_SCHEDULE)."
	}
	var b strings.Builder
	b.WriteString("🗓️ **Horário fixo**")
	if s.Timezone != "" {
		fmt.Fprintf(&b, " (%s)", s.Timezone)
	}
	b.WriteString(":\n")

	order := []string{"Default"}
	order = append(order, weekdayNames...)
	shown := map[string]bool{}
	for _, key := range order {
		blocks, ok := s.Days[key]
		if !ok || shown[key] {
			continue
		}
		shown[key] = true
		label := key
		if key == "Default" {
			label = "Todos os dias"
		}
		fmt.Fprintf(&b, "- **%s:**\n", label)
		for _, blk := range blocks {
			name := blk.Label
			if name == "" {
				name = "—"
			}
			fmt.Fprintf(&b, "  - %s–%s %s\n", blk.Start, blk.End, name)
		}
	}
	return strings.TrimSpace(b.String())
}
