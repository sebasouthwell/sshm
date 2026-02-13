package history

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Ranker provides history-based ranking for entries
type Ranker struct {
	historyFile string
	events      []*Event
	loaded      bool
}

// NewRanker creates a new history ranker
func NewRanker(historyFile string) *Ranker {
	if historyFile == "" {
		home, _ := os.UserHomeDir()
		historyFile = filepath.Join(home, ".ssh", "inventory.d", "history.jsonl")
	}

	return &Ranker{
		historyFile: historyFile,
		events:      []*Event{},
	}
}

// Load loads history events
func (r *Ranker) Load() error {
	if r.loaded {
		return nil
	}

	data, err := ioutil.ReadFile(r.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			r.loaded = true
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip invalid lines
		}
		r.events = append(r.events, &event)
	}

	r.loaded = true
	return nil
}

// EntryScore represents a score for an entry
type EntryScore struct {
	Alias      string
	Recent     float64 // Score for recent usage (last 24h)
	Frequent   float64 // Score for frequent usage (last 30 days)
	TotalScore float64 // Combined score
}

// RankEntries ranks entries by usage history
func (r *Ranker) RankEntries(aliases []string) ([]EntryScore, error) {
	if err := r.Load(); err != nil {
		return nil, err
	}

	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)

	scores := make(map[string]*EntryScore)

	// Initialize scores
	for _, alias := range aliases {
		scores[alias] = &EntryScore{
			Alias: alias,
		}
	}

	// Calculate scores from events
	for _, event := range r.events {
		if event.Status != "ok" {
			continue // Only count successful connections
		}

		score, exists := scores[event.Alias]
		if !exists {
			continue
		}

		eventTime := time.Unix(event.TS, 0)

		// Recent score (last 24h)
		if eventTime.After(dayAgo) {
			score.Recent += 1.0
		}

		// Frequent score (last 30 days)
		if eventTime.After(monthAgo) {
			score.Frequent += 1.0
		}
	}

	// Calculate total scores (weighted)
	var result []EntryScore
	for _, score := range scores {
		// Weight: recent (2x) + frequent (1x)
		score.TotalScore = score.Recent*2.0 + score.Frequent*1.0
		result = append(result, *score)
	}

	// Sort by total score (descending)
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalScore > result[j].TotalScore
	})

	return result, nil
}

// IsRecent checks if an alias was used recently (last 24h)
func (r *Ranker) IsRecent(alias string) (bool, error) {
	if err := r.Load(); err != nil {
		return false, err
	}

	dayAgo := time.Now().Add(-24 * time.Hour)
	for _, event := range r.events {
		if event.Alias == alias && event.Status == "ok" {
			eventTime := time.Unix(event.TS, 0)
			if eventTime.After(dayAgo) {
				return true, nil
			}
		}
	}

	return false, nil
}

// IsFrequent checks if an alias is frequently used (top 10 in last 30 days)
func (r *Ranker) IsFrequent(alias string) (bool, error) {
	if err := r.Load(); err != nil {
		return false, err
	}

	// Count usage in last 30 days
	monthAgo := time.Now().Add(-30 * 24 * time.Hour)
	usageCount := make(map[string]int)

	for _, event := range r.events {
		if event.Status != "ok" {
			continue
		}
		eventTime := time.Unix(event.TS, 0)
		if eventTime.After(monthAgo) {
			usageCount[event.Alias]++
		}
	}

	// Find top 10
	type usagePair struct {
		alias string
		count int
	}
	var pairs []usagePair
	for a, c := range usageCount {
		pairs = append(pairs, usagePair{alias: a, count: c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})

	// Check if alias is in top 10
	for i, pair := range pairs {
		if i >= 10 {
			break
		}
		if pair.alias == alias {
			return true, nil
		}
	}

	return false, nil
}

// GetUsageCount returns the usage count for an alias in the last 30 days
func (r *Ranker) GetUsageCount(alias string) (int, error) {
	if err := r.Load(); err != nil {
		return 0, err
	}

	monthAgo := time.Now().Add(-30 * 24 * time.Hour)
	count := 0

	for _, event := range r.events {
		if event.Alias == alias && event.Status == "ok" {
			eventTime := time.Unix(event.TS, 0)
			if eventTime.After(monthAgo) {
				count++
			}
		}
	}

	return count, nil
}
