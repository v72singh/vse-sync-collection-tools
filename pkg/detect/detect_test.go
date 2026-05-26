// SPDX-License-Identifier: GPL-2.0-or-later

package detect

import "testing"

const legacyEthtoolTsInfo = `Time stamping parameters for ens6f0:
Capabilities:
	hardware-transmit
	software-transmit
	hardware-receive
	software-receive
	software-system-clock
	hardware-raw-clock
PTP Hardware Clock: 0
Hardware Transmit Timestamp Modes:
	off
	on
Hardware Receive Filter Modes:
	none
	all
`

const providerEthtoolTsInfo = `Time stamping parameters for ens3f0:
Capabilities:
	hardware-transmit
	software-transmit
	hardware-receive
	software-receive
	software-system-clock
	hardware-raw-clock
Hardware timestamp provider index: 0
Hardware timestamp provider qualifier: Precise (IEEE 1588 quality)
Hardware Transmit Timestamp Modes:
	off
	on
Hardware Receive Filter Modes:
	none
	all
`

func TestParsePTPClockIndexFromEthtool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		out     string
		want    int
		wantErr bool
	}{
		{name: "legacy phc index", out: legacyEthtoolTsInfo, want: 0},
		{name: "provider index fallback", out: providerEthtoolTsInfo, want: 0},
		{name: "provider index only", out: "Hardware timestamp provider index: 0\n", want: 0},
		{
			name: "no phc",
			out: `Capabilities:
	hardware-transmit
PTP Hardware Clock: none
`,
			wantErr: true,
		},
		{name: "missing phc info", out: "Capabilities:\n\tsoftware-transmit\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePTPClockIndexFromEthtool(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsePTPClockIndexFromEthtool() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePTPClockIndexFromEthtool() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parsePTPClockIndexFromEthtool() = %d, want %d", got, tt.want)
			}
		})
	}
}
