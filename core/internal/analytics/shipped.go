package analytics

import (
	"time"

	"github.com/Hypership-Software/aftcast/internal/schema"
)

type ShippedProfile struct {
	Eligible      int
	Shipped       int
	Rate          float64
	TrackingSince time.Time
}

func DeliveryEligible(s SessionStat) bool {
	return s.CaptureVersion >= schema.DeliverySignalVersion && (s.FilesChanged > 0 || s.Shipped)
}

func ShippingProfile(sessions []SessionStat) ShippedProfile {
	var out ShippedProfile
	for _, s := range sessions {
		if s.CaptureVersion >= schema.DeliverySignalVersion {
			if started, err := time.Parse(time.RFC3339Nano, s.Started); err == nil && (out.TrackingSince.IsZero() || started.Before(out.TrackingSince)) {
				out.TrackingSince = started
			}
		}
		if !DeliveryEligible(s) {
			continue
		}
		out.Eligible++
		if s.Shipped {
			out.Shipped++
		}
	}
	if out.Eligible > 0 {
		out.Rate = float64(out.Shipped) / float64(out.Eligible)
	}
	return out
}
