// SPDX-License-Identifier: GPL-2.0-or-later
//
// Telecom profile clock parser and formatter derived from PMC outputs.
// This extracts clock class, clock accuracy and maps configured clock type
// to Telecom profile labels (T-GM or T-BC), taking reference from type.go.
package devices

import (
	"strings"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/callbacks"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/constants"
)

// TelecomClockProfile contains a minimal subset of PMCInfo formatted for analysers.
type TelecomClockProfile struct {
	Timestamp     string `json:"timestamp"`
	TelecomType   string `json:"telecom_clock_type"` // either "T-GM" or "T-BC"
	ClockClass    int    `json:"clock_class"`
	ClockAccuracy string `json:"clock_accuracy"`
}

// BuildTelecomClockProfileFromPMC converts PMCInfo into TelecomClockProfile with a mapped telecom type.
func BuildTelecomClockProfileFromPMC(pmc *PMCInfo, configuredClockType string) *TelecomClockProfile {
	telecomType := ""
	switch strings.ToUpper(configuredClockType) {
	case constants.ClockTypeGM:
		telecomType = "T-GM"
	case constants.ClockTypeBC:
		telecomType = "T-BC"
	default:
		telecomType = configuredClockType
	}

	return &TelecomClockProfile{
		Timestamp:     pmc.Timestamp,
		TelecomType:   telecomType,
		ClockClass:    pmc.ClockClass,
		ClockAccuracy: pmc.ClockAccuracy,
	}
}

// GetAnalyserFormat returns the analyser JSON payload.
func (p *TelecomClockProfile) GetAnalyserFormat() ([]*callbacks.AnalyserFormatType, error) {
	formatted := callbacks.AnalyserFormatType{
		ID: "ptp/telecom-clock-profile",
		Data: map[string]any{
			"timestamp":           p.Timestamp,
			"telecom_clock_type":  p.TelecomType,
			"clock_class":         p.ClockClass,
			"clock_accuracy":      p.ClockAccuracy,
		},
	}
	return []*callbacks.AnalyserFormatType{&formatted}, nil
}
