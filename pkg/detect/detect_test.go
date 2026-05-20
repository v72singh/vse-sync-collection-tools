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

const gnrdTs2phcConfig = `#profile: gnrd-tgm_grandmaster

[nmea]
ts2phc.master 1
[global]
use_syslog 0
[eno8703np0]
ts2phc.master 0
[enp108s0f0np0]
ts2phc.master 0
[enp110s0f0np0]
ts2phc.master 0
`

func TestGnrdTs2phcConfigParsing(t *testing.T) {
	t.Parallel()

	config, netdevOrder, err := parseConfigWithOrder(gnrdTs2phcConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !nmeaSectionIsMaster(config) {
		t.Fatal("expected [nmea] to be ts2phc master")
	}
	wantOrder := []string{"eno8703np0", "enp108s0f0np0", "enp110s0f0np0"}
	if len(netdevOrder) != len(wantOrder) {
		t.Fatalf("netdev order %v, want %v", netdevOrder, wantOrder)
	}
	for i := range wantOrder {
		if netdevOrder[i] != wantOrder[i] {
			t.Fatalf("netdev order[%d]=%s want %s", i, netdevOrder[i], wantOrder[i])
		}
	}
	for _, iface := range wantOrder {
		if sectionHasTs2phcMaster(config[iface]) {
			t.Fatalf("%s should not be master", iface)
		}
	}
}

func TestFillMissingPTPDevices(t *testing.T) {
	t.Parallel()

	config, netdevOrder, err := parseConfigWithOrder(gnrdTs2phcConfig)
	if err != nil {
		t.Fatal(err)
	}
	detected := []DetectedInterface{
		{Name: "eno8703np0", Primary: true},
		{Name: "enp108s0f0np0"},
		{Name: "enp110s0f0np0"},
	}
	result := fillMissingPTPDevices(detected, netdevOrder, true)
	if result[0].PTPClockDevicePath != "/dev/ptp0" {
		t.Fatalf("eno8703np0 got %q", result[0].PTPClockDevicePath)
	}
	if result[1].PTPClockDevicePath != "/dev/ptp1" {
		t.Fatalf("enp108 got %q", result[1].PTPClockDevicePath)
	}
	if result[2].PTPClockDevicePath != "/dev/ptp2" {
		t.Fatalf("enp110 got %q", result[2].PTPClockDevicePath)
	}
	if !nmeaSectionIsMaster(config) {
		t.Fatal("expected nmea master for fill test")
	}
}

func TestPtpIndexFromTs2phcSinkLine(t *testing.T) {
	t.Parallel()

	match := ts2phcSinkPTPIndex.FindStringSubmatch("PPS sink eno8703np0 has ptp index 0")
	if len(match) < 2 || match[1] != "0" {
		t.Fatalf("unexpected match %v", match)
	}
}

func TestApplyNmeaMasterPrimary(t *testing.T) {
	t.Parallel()

	config, _, err := parseConfigWithOrder(gnrdTs2phcConfig)
	if err != nil {
		t.Fatal(err)
	}

	detected := []DetectedInterface{
		{Name: "enp110s0f0np0", PTPClockDevicePath: "/dev/ptp2"},
		{Name: "eno8703np0", PTPClockDevicePath: "/dev/ptp0"},
		{Name: "enp108s0f0np0", PTPClockDevicePath: "/dev/ptp1"},
	}

	result := applyNmeaMasterPrimary(nil, detected, config)

	primaryCount := 0
	var primaryName string
	for _, iface := range result {
		if iface.Primary {
			primaryCount++
			primaryName = iface.Name
		}
	}
	if primaryCount != 1 {
		t.Fatalf("expected 1 primary, got %d", primaryCount)
	}
	if primaryName != "eno8703np0" {
		t.Fatalf("expected eno8703np0 primary, got %s", primaryName)
	}
}
