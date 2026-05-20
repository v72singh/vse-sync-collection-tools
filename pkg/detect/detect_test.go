// SPDX-License-Identifier: GPL-2.0-or-later

package detect

import (
	"errors"
	"testing"
)

func TestPtpClockFromEthtool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{
			name: "standard phc line",
			output: `Time stamping parameters for eth0:
PTP Hardware Clock: 2
`,
			want: "/dev/ptp2",
		},
		{
			name:    "missing phc line",
			output:  "Time stamping parameters for bond0:\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ptpClockFromEthtool(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, errNoPTPClockDevice) {
					t.Fatalf("got err %v, want %v", err, errNoPTPClockDevice)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePTPClockDevicePath(t *testing.T) {
	t.Parallel()

	if got := resolvePTPClockDevicePath("/dev/ptp3"); got != "/dev/ptp3" {
		t.Fatalf("got %q", got)
	}
	if got := resolvePTPClockDevicePath("  /dev/ptp0  "); got != "/dev/ptp0" {
		t.Fatalf("got %q", got)
	}
	if got := resolvePTPClockDevicePath("eno1"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestIsSkippedConfigSection(t *testing.T) {
	t.Parallel()

	for _, section := range []string{"global", "nmea", "GLOBAL"} {
		if !isSkippedConfigSection(section) {
			t.Fatalf("%q should be skipped", section)
		}
	}
	if isSkippedConfigSection("eno8303") {
		t.Fatal("interface section should not be skipped")
	}
}
