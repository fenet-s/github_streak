package service

import (
	"testing"
	"time"
)

// dateOnly creates a UTC time at midnight for the given y-m-d.
func dateOnly(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestCalculateStreak_Empty(t *testing.T) {
	current, longest, last := CalculateStreak(nil)
	if current != 0 || longest != 0 || !last.IsZero() {
		t.Errorf("expected (0, 0, zero) for empty input, got (%d, %d, %v)", current, longest, last)
	}
}

func TestCalculateStreak_SingleToday(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	current, longest, _ := CalculateStreak([]time.Time{today})
	if current != 1 {
		t.Errorf("expected current=1, got %d", current)
	}
	if longest != 1 {
		t.Errorf("expected longest=1, got %d", longest)
	}
}

func TestCalculateStreak_ConsecutiveEndingToday(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dates := []time.Time{
		today,
		today.Add(-24 * time.Hour),
		today.Add(-48 * time.Hour),
		today.Add(-72 * time.Hour),
	}
	current, longest, _ := CalculateStreak(dates)
	if current != 4 {
		t.Errorf("expected current=4, got %d", current)
	}
	if longest != 4 {
		t.Errorf("expected longest=4, got %d", longest)
	}
}

func TestCalculateStreak_BrokenStreak(t *testing.T) {
	// Spec example:
	// Sep 10 ✅, Sep 11 ✅, Sep 12 ✅, Sep 13 ✅, Sep 14 ❌
	// Current streak should be 0 (missed yesterday)
	yesterday := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	dates := []time.Time{
		yesterday.Add(-24 * time.Hour),      // 2 days ago
		yesterday.Add(-48 * time.Hour),      // 3 days ago
		yesterday.Add(-72 * time.Hour),      // 4 days ago
		yesterday.Add(-96 * time.Hour),      // 5 days ago
	}
	current, longest, _ := CalculateStreak(dates)
	if current != 0 {
		t.Errorf("expected current=0 (missed yesterday), got %d", current)
	}
	if longest != 4 {
		t.Errorf("expected longest=4, got %d", longest)
	}
}

func TestCalculateStreak_GapInMiddle(t *testing.T) {
	// Spec example:
	// Sep 10 ✅, Sep 11 ✅, Sep 12 ❌, Sep 13 ✅, Sep 14 ✅
	// Current streak: 2 days
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dates := []time.Time{
		today,
		today.Add(-24 * time.Hour),
		// gap: today.Add(-48 * time.Hour) missing
		today.Add(-72 * time.Hour),
		today.Add(-96 * time.Hour),
	}
	current, _, _ := CalculateStreak(dates)
	if current != 2 {
		t.Errorf("expected current=2, got %d", current)
	}
}

func TestCalculateStreak_LongestIsOlder(t *testing.T) {
	// Current streak: 1, but had a longer streak of 5 in the past
	today := time.Now().UTC().Truncate(24 * time.Hour)
	pastBase := dateOnly(2025, 1, 20)
	dates := []time.Time{
		today,
		// gap
		pastBase,
		pastBase.Add(-24 * time.Hour),
		pastBase.Add(-48 * time.Hour),
		pastBase.Add(-72 * time.Hour),
		pastBase.Add(-96 * time.Hour),
	}
	current, longest, _ := CalculateStreak(dates)
	if current != 1 {
		t.Errorf("expected current=1, got %d", current)
	}
	if longest != 5 {
		t.Errorf("expected longest=5, got %d", longest)
	}
}
