package plans

import "time"

const Unlimited = -1

const (
	Free     = "free"
	Pro      = "pro"
	Internal = "internal"
)

type Plan struct {
	Name                       string
	PriceUSD                   int
	MaxJobs                    int
	MaxCandidatesPerJob        int
	OutreachCandidatesLifetime int
	OutreachGenerationsPerDay  int
	RescanCandidatesLifetime   int
	RescanPerCandidatePerDay   int
	MinInterval                time.Duration
	ExportFormats              []string
	PublicLinkOutreachLimit    int
}

var catalog = map[string]Plan{
	Free: {
		Name:                       Free,
		PriceUSD:                   0,
		MaxJobs:                    1,
		MaxCandidatesPerJob:        20,
		OutreachCandidatesLifetime: 3,
		OutreachGenerationsPerDay:  3,
		RescanCandidatesLifetime:   3,
		RescanPerCandidatePerDay:   1,
		MinInterval:                10 * time.Minute,
		ExportFormats:              []string{"json", "csv"},
		PublicLinkOutreachLimit:    3,
	},
	Pro: {
		Name:                       Pro,
		PriceUSD:                   59,
		MaxJobs:                    4,
		MaxCandidatesPerJob:        100,
		OutreachCandidatesLifetime: Unlimited,
		OutreachGenerationsPerDay:  Unlimited,
		RescanCandidatesLifetime:   Unlimited,
		RescanPerCandidatePerDay:   3,
		MinInterval:                time.Minute,
		ExportFormats:              []string{"json", "csv", "excel", "pdf"},
		PublicLinkOutreachLimit:    3,
	},
	Internal: {
		Name:                       Internal,
		PriceUSD:                   0,
		MaxJobs:                    Unlimited,
		MaxCandidatesPerJob:        20,
		OutreachCandidatesLifetime: Unlimited,
		OutreachGenerationsPerDay:  Unlimited,
		RescanCandidatesLifetime:   Unlimited,
		RescanPerCandidatePerDay:   Unlimited,
		MinInterval:                0,
		ExportFormats:              []string{"json", "csv", "excel", "pdf"},
		PublicLinkOutreachLimit:    3,
	},
}

func For(name string) Plan {
	if p, ok := catalog[name]; ok {
		return p
	}
	return catalog[Free]
}

func (p Plan) AllowsExport(format string) bool {
	for _, f := range p.ExportFormats {
		if f == format {
			return true
		}
	}
	return false
}

func Exceeds(limit, used int) bool {
	if limit == Unlimited {
		return false
	}
	return used >= limit
}
