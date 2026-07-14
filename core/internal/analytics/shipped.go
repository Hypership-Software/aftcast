package analytics

import "github.com/Hypership-Software/atlas/internal/schema"

type ShippedProfile struct {
	Eligible int
	Shipped  int
	Rate     float64
}

func DeliveryEligible(s SessionStat) bool {
	return s.CaptureVersion >= schema.DeliverySignalVersion && (s.FilesChanged > 0 || s.Shipped)
}

func ShippingProfile(sessions []SessionStat) ShippedProfile {
	var out ShippedProfile
	for _, s := range sessions {
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
