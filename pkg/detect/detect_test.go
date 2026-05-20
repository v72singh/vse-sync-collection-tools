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
			name: "phc none is ignored",
			output: `PTP Hardware Clock: none
`,
			wantErr: true,
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

func TestIsValidPTPClockIndex(t *testing.T) {
	t.Parallel()

	if !isValidPTPClockIndex("0") || !isValidPTPClockIndex("12") {
		t.Fatal("numeric indices should be valid")
	}
	if isValidPTPClockIndex("none") || isValidPTPClockIndex("") || isValidPTPClockIndex("n/a") {
		t.Fatal("non-numeric indices should be invalid")
	}
}
