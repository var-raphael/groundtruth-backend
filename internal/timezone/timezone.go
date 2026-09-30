package timezone

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const geocodeURL = "https://geocoding-api.open-meteo.com/v1/search"

var httpClient = &http.Client{Timeout: 5 * time.Second}

type geocodeResponse struct {
	Results []struct {
		Timezone string `json:"timezone"`
	} `json:"results"`
}

// Derive resolves a city to a UTC offset string (e.g. "+01:00") using the
// Open-Meteo geocoding API, which wraps GeoNames data and needs no API key.
// Returns "" if the city can't be resolved (unknown city, network failure,
// or timeout) — callers should treat an empty result as "timezone unknown"
// rather than an error, since this feeds filtering/display, not a hard
// computation path.
func Derive(city string) string {
	if city == "" {
		return ""
	}

	reqURL := fmt.Sprintf("%s?name=%s&count=1", geocodeURL, url.QueryEscape(city))
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var result geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	if len(result.Results) == 0 {
		return ""
	}

	return ianaToOffset(result.Results[0].Timezone)
}

// ianaToOffset converts an IANA timezone name (e.g. "Africa/Lagos") to a
// UTC offset string (e.g. "+01:00"), using Go's stdlib tzdata so it
// accounts for the current date's DST rules automatically.
func ianaToOffset(iana string) string {
	loc, err := time.LoadLocation(iana)
	if err != nil {
		return ""
	}

	_, offsetSeconds := time.Now().In(loc).Zone()
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60

	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

// OverlapHours estimates working-hour overlap between two UTC offsets,
// assuming an 8-hour workday (9am-5pm local) on each side. Offsets are
// strings like "+01:00" or "-05:00", the same format Derive returns.
// Returns 0 if either offset is empty/unparseable rather than erroring,
// since this feeds filtering/display, not a hard computation path.
func OverlapHours(offsetA, offsetB string) int {
	a, okA := parseOffsetHours(offsetA)
	b, okB := parseOffsetHours(offsetB)
	if !okA || !okB {
		return 0
	}

	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	overlap := 8 - diff
	if overlap < 0 {
		overlap = 0
	}
	return overlap
}

// BestOverlapHours returns the highest overlap between candidateOffset and
// any of jobOffsets, since a candidate qualifies if they overlap with at
// least one of the job's timezones.
func BestOverlapHours(candidateOffset string, jobOffsets []string) int {
	best := 0
	for _, jobOffset := range jobOffsets {
		if h := OverlapHours(candidateOffset, jobOffset); h > best {
			best = h
		}
	}
	return best
}

func parseOffsetHours(offset string) (int, bool) {
	if len(offset) < 6 {
		return 0, false
	}
	sign := 1
	if offset[0] == '-' {
		sign = -1
	} else if offset[0] != '+' {
		return 0, false
	}

	hours := 0
	if _, err := fmt.Sscanf(offset[1:3], "%d", &hours); err != nil {
		return 0, false
	}
	return sign * hours, true
}
