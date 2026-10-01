package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const DEFAULT_TIMEZONE = "America/Sao_Paulo"

type WorkBlock struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"`
}

type Schedule struct {
	Timezone string                 `json:"timezone"`
	Days     map[string][]WorkBlock `json:"days"`
}

func weekdayNames() []string {
	return []string{
		"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
	}
}

func defaultBlocks() []WorkBlock {
	return []WorkBlock{
		{Start: "09:00", End: "09:30", Label: "Daily"},
		{Start: "09:30", End: "12:00", Label: "Manhã"},
		{Start: "13:00", End: "18:00", Label: "Tarde"},
	}
}

func LoadSchedule(raw string) *Schedule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return buildDefaultSchedule()
	}

	s := Schedule{}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return buildDefaultSchedule()
	}

	if s.Days == nil {
		s.Days = map[string][]WorkBlock{}
	}
	if s.Timezone == "" {
		s.Timezone = DEFAULT_TIMEZONE
	}

	return &s
}

func buildDefaultSchedule() *Schedule {
	blocks := defaultBlocks()
	s := &Schedule{Timezone: DEFAULT_TIMEZONE, Days: map[string][]WorkBlock{}}
	for _, d := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday"} {
		s.Days[d] = blocks
	}
	return s
}

func (s *Schedule) BlocksFor(t time.Time) []WorkBlock {
	if s == nil {
		return nil
	}

	blocks, ok := s.Days[t.Weekday().String()]
	if ok {
		return blocks
	}

	blocks, ok = s.Days["Default"]
	if ok {
		return blocks
	}

	return nil
}

func (s *Schedule) location() *time.Location {
	if s == nil {
		return time.Local
	}

	if s.Timezone == "" {
		return time.Local
	}

	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Local
	}

	return loc
}

func validHour(h int) bool {
	if h < 0 {
		return false
	}
	return h <= 23
}

func validMinute(m int) bool {
	if m < 0 {
		return false
	}
	return m <= 59
}

func blockTime(day time.Time, hhmm string) (time.Time, bool) {
	h := 0
	m := 0
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return time.Time{}, false
	}

	if !validHour(h) {
		return time.Time{}, false
	}

	if !validMinute(m) {
		return time.Time{}, false
	}

	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location()), true
}

func dayLabel(key string) string {
	if key == "Default" {
		return "Todos os dias"
	}
	return key
}

func formatScheduleSection(s *Schedule) string {
	if s == nil {
		return SCHEDULE_MISSING
	}

	b := strings.Builder{}
	b.WriteString("🗓️ **Horário fixo**")
	if s.Timezone != "" {
		fmt.Fprintf(&b, " (%s)", s.Timezone)
	}
	b.WriteString(":\n")

	order := []string{"Default"}
	order = append(order, weekdayNames()...)

	shown := map[string]bool{}
	for _, key := range order {
		blocks, ok := s.Days[key]
		if !ok {
			continue
		}
		if shown[key] {
			continue
		}
		shown[key] = true

		fmt.Fprintf(&b, "- **%s:**\n", dayLabel(key))
		for _, blk := range blocks {
			fmt.Fprintf(&b, "  - %s–%s %s\n", blk.Start, blk.End, blockLabel(blk))
		}
	}

	return strings.TrimSpace(b.String())
}
