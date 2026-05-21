// SPDX-License-Identifier: GPL-2.0-or-later

package devices

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/callbacks"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/clients"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/fetcher"
)

var states = map[string]string{
	"unknown":       "-1",
	"invalid":       "0",
	"freerun":       "1",
	"locked":        "2",
	"locked-ho-acq": "3",
	"holdover":      "4",
}

const (
	maxNetlinkPinProbe = 64

	OnePPSLabel = "GNSS-1PPS"
	SMA1Label   = "SMA1"
	SMA2Label   = "SMA2"

	OnePPSSubtype  = "dpll"
	SMA1Subtype    = "dpll-sma1"
	UnknownSubtype = "unknown"

	InputDirection = "input"
	ConnectedState = "connected"

	EECOffsetParentID      = 0
	PPSOffesetParentID     = 1
	DPLLPhaseOffsetDivider = 1000
)

type DevNetlinkDPLLInfo struct {
	PinType   string
	Timestamp string `fetcherKey:"date"       json:"timestamp"`
	EECState  string `fetcherKey:"eec"        json:"eecstate"`
	PPSState  string `fetcherKey:"pps"        json:"state"`
	PPSOffset int64  `fetcherKey:"pps_offset" json:"terror"`
	EECOffset int64  `fetcherKey:"eec_offset" json:"eecterror"`
}

func convertNetlinkOffset(offset int64) float64 {
	// Convert to nano seconds with 3 decimal places
	return float64(int64(math.Round(float64(offset/DPLLPhaseOffsetDivider)))) / 1000 //nolint:gomnd,mnd // this is just for decimal places
}

// AnalyserJSON returns the json expected by the analysers
func (dpllInfo *DevNetlinkDPLLInfo) GetAnalyserFormat() ([]*callbacks.AnalyserFormatType, error) {
	subType := UnknownSubtype
	switch dpllInfo.PinType {
	case OnePPSLabel:
		subType = OnePPSSubtype
	case SMA1Label:
		subType = SMA1Subtype
	}

	formatted := callbacks.AnalyserFormatType{
		ID: fmt.Sprintf("%s/time-error", subType),
		Data: map[string]any{
			"timestamp": dpllInfo.Timestamp,
			"eecstate":  dpllInfo.EECState,
			"state":     dpllInfo.PPSState,
			"terror":    convertNetlinkOffset(dpllInfo.PPSOffset),
			"eecterror": convertNetlinkOffset(dpllInfo.EECOffset),
		},
	}
	return []*callbacks.AnalyserFormatType{&formatted}, nil
}

type NetlinkStateEntry struct {
	LockStatus string `json:"lock-status"` //nolint:tagliatelle // not my choice
	Driver     string `json:"module-name"` //nolint:tagliatelle // not my choice
	ClockType  string `json:"type"`        //nolint:tagliatelle // not my choice
	ClockID    uint64 `json:"clock-id"`    //nolint:tagliatelle // not my choice
	ID         int    `json:"id"`          //nolint:tagliatelle // not my choice
}

// # Example output
// [{'clock-id': 5799633565435100136,
//   'id': 0,
//   'lock-status': 'locked-ho-acq',
//   'mode': 'automatic',
//   'mode-supported': ['automatic'],
//   'module-name': 'ice',
//   'type': 'eec'},
//  {'clock-id': 5799633565435100136,
//   'id': 1,
//   'lock-status': 'locked-ho-acq',
//   'mode': 'automatic',
//   'mode-supported': ['automatic'],
//   'module-name': 'ice',
//   'type': 'pps'}]

type NetlinkPin struct {
	Type                 string                            `json:"type"`                //nolint:tagliatelle // not my choice
	ModuleName           string                            `json:"module-name"`         //nolint:tagliatelle // not my choice
	Label                string                            `json:"board-label"`         //nolint:tagliatelle // not my choice
	Capabilities         []string                          `json:"capabilities"`        //nolint:tagliatelle // not my choice
	FrequenciesSupported []*NetlinkFrequencySupportedRange `json:"frequency-supported"` //nolint:tagliatelle // not my choice
	ParentDevices        []*NetlinkParentDevice            `json:"parent-device"`       //nolint:tagliatelle // not my choice
	ParentPins           []*NetlinkParentPin               `json:"parent-pin"`          //nolint:tagliatelle // not my choice
	ClockID              uint64                            `json:"clock-id"`            //nolint:tagliatelle // not my choice
	Frequency            uint64                            `json:"frequency"`           //nolint:tagliatelle // not my choice
	ID                   int32                             `json:"id"`                  //nolint:tagliatelle // not my choice
	PhaseAdjust          int32                             `json:"phase-adjust"`        //nolint:tagliatelle // not my choice
	PhaseAdjustMax       int32                             `json:"phase-adjust-max"`    //nolint:tagliatelle // not my choice
	PhaseAdjustMin       int32                             `json:"phase-adjust-min"`    //nolint:tagliatelle // not my choice
}

type NetlinkParentDevice struct {
	Direction   string `json:"direction"`    //nolint:tagliatelle // not my choice
	State       string `json:"state"`        //nolint:tagliatelle // not my choice
	ParentID    int    `json:"parent-id"`    //nolint:tagliatelle // not my choice
	PhaseOffset int64  `json:"phase-offset"` //nolint:tagliatelle // not my choice
	Prio        int    `json:"prio"`         //nolint:tagliatelle // not my choice
}

type NetlinkParentPin struct {
	State    string `json:"state"`     //nolint:tagliatelle // not my choice
	ParentID int32  `json:"parent-id"` //nolint:tagliatelle // not my choice
}

type NetlinkFrequencySupportedRange struct {
	Max int32 `json:"frequency-max"` //nolint:tagliatelle // not my choice
	Min int32 `json:"frequency-min"` //nolint:tagliatelle // not my choice
}

// # Example output
// {
// 	'board-label': 'GNSS-1PPS',
// 	'capabilities': 6,
// 	'clock-id': 5799633565433967608,
// 	'frequency': 1,
// 	'frequency-supported': [
// 		{
// 			'frequency-max': 1,
// 			'frequency-min': 1
// 		}
// 	],
// 	'id': 6,
// 	'module-name': 'ice',
// 	'parent-device': [
// 		{
// 			'direction': 'input',
// 			'parent-id': 0,
// 			'phase-offset': 406616064733390,
// 			'prio': 0,
// 			'state': 'connected'
// 		},
// 		{
// 			'direction': 'input',
// 			'parent-id': 1,
// 			'phase-offset': -1870360,
// 			'prio': 0,
// 			'state': 'connected'
// 		}
// 	],
// 	'phase-adjust': 0,
// 	'phase-adjust-max': 16723,
// 	'phase-adjust-min': -16723,
// 	'type': 'gnss'
// },

var (
	dpllNetlinkFetcher map[uint64]*fetcher.Fetcher
	dpllClockIDFetcher map[string]*fetcher.Fetcher
)

func init() {
	dpllNetlinkFetcher = make(map[uint64]*fetcher.Fetcher)
	dpllClockIDFetcher = make(map[string]*fetcher.Fetcher)
}

func buildPostProcessDPLLNetlink(clockID uint64) fetcher.PostProcessFuncType {
	return func(result map[string]string) (map[string]any, error) {
		processedResult := make(map[string]any)

		entries := make([]NetlinkStateEntry, 0)
		deviceOut := strings.TrimSpace(result["dpll-netlink-device"])
		if deviceOut != "" && deviceOut[0] == '[' {
			err := json.Unmarshal([]byte(deviceOut), &entries)
			if err != nil {
				log.Errorf("Failed to unmarshal netlink device output: %s", err.Error())
			}
		} else if deviceOut != "" {
			var entry NetlinkStateEntry
			if err := json.Unmarshal([]byte(deviceOut), &entry); err != nil {
				log.Errorf("Failed to unmarshal netlink device output: %s", err.Error())
			} else {
				entries = append(entries, entry)
			}
		}

		log.Debug("entries: ", entries)
		for _, entry := range entries {
			if entry.ClockID == clockID {
				state, ok := states[entry.LockStatus]
				if !ok {
					log.Errorf("Unknown state: %s", state)
					state = "-1"
				}
				processedResult[entry.ClockType] = state
			}
		}
		pin := NetlinkPin{}
		err := json.Unmarshal([]byte(result["dpll-netlink-offset"]), &pin)
		if err != nil {
			log.Errorf("Failed to unmarshal netlink pin output: %s", err.Error())
		}

		for _, parentPin := range pin.ParentDevices {
			switch parentPin.ParentID % 2 {
			case EECOffsetParentID:
				processedResult["ecc_offset"] = parentPin.PhaseOffset
			case PPSOffesetParentID:
				processedResult["pps_offset"] = parentPin.PhaseOffset
			}
		}
		return processedResult, nil
	}
}

// BuildDPLLNetlinkDeviceFetcher popluates the fetcher required for
// collecting the DPLLInfo
func BuildDPLLNetlinkDeviceFetcher(params NetlinkParameters) error { //nolint:dupl // Further dedup risks be too abstract or fragile
	fetcherInst, err := fetcher.FetcherFactory(
		[]*clients.Cmd{dateCmd},
		[]fetcher.AddCommandArgs{
			{
				Key: "dpll-netlink-device",
				// device-get dump fails on GNRD (KeyError: 12); try per-device queries.
				Command: "sh -c 'for i in 0 1 2 3; do OUT=$(/linux/tools/net/ynl/cli.py --spec " +
					"/linux/Documentation/netlink/specs/dpll.yaml --do device-get --json " +
					"\"{\\\"id\\\": $i}\" 2>/dev/null | python3 /root/custom_scripts/json_encoder.py); " +
					"if [ -n \"$OUT\" ]; then echo \"$OUT\"; break; fi; done'",
				Trim: true,
			},
			{
				Key: "dpll-netlink-offset",
				Command: fmt.Sprintf(
					"/linux/tools/net/ynl/cli.py --spec /linux/Documentation/netlink/specs/dpll.yaml --do pin-get --json %s | "+
						"python3 /root/custom_scripts/json_encoder.py",
					fmt.Sprintf("'{\"id\": %d}'", params.OffsetPin),
				),
				Trim: true,
			},
		},
	)
	if err != nil {
		log.Errorf("failed to create fetcher for dpll netlink: %s", err.Error())
		return fmt.Errorf("failed to create fetcher for dpll netlink: %w", err)
	}
	dpllNetlinkFetcher[params.ClockID] = fetcherInst
	fetcherInst.SetPostProcessor(buildPostProcessDPLLNetlink(params.ClockID))
	return nil
}

// GetDevDPLLInfo returns the device DPLL info for an interface.
func GetDevDPLLNetlinkInfo(ctx clients.ExecContext, params NetlinkParameters) (*DevNetlinkDPLLInfo, error) {
	dpllInfo := &DevNetlinkDPLLInfo{PinType: params.PinType}
	fetcherInst, fetchedInstanceOk := dpllNetlinkFetcher[params.ClockID]
	if !fetchedInstanceOk {
		err := BuildDPLLNetlinkDeviceFetcher(params)
		if err != nil {
			return dpllInfo, err
		}
		fetcherInst, fetchedInstanceOk = dpllNetlinkFetcher[params.ClockID]
		if !fetchedInstanceOk {
			return dpllInfo, errors.New("failed to create fetcher for DPLLInfo using netlink interface")
		}
	}
	err := fetcherInst.Fetch(ctx, dpllInfo)
	if err != nil {
		log.Debugf("failed to fetch dpllInfo  via netlink: %s", err.Error())
		return dpllInfo, fmt.Errorf("failed to fetch dpllInfo via netlink: %w", err)
	}
	return dpllInfo, nil
}

func BuildNetlinkInfoFetcher(interfaceName string) error {
	fetcherInst, err := fetcher.FetcherFactory(
		[]*clients.Cmd{dateCmd},
		[]fetcher.AddCommandArgs{
			{
				Key: "dpll-netlink-clock-id",
				// Bash $((16#...)) overflows to negative int64 on E825; use Python for uint64.
				Command: fmt.Sprintf(
					`export IFNAME=%s; export BUSID=$(readlink /sys/class/net/$IFNAME/device | xargs basename | cut -d ':' -f 2,3);`+
						` export SERIAL=$(lspci -v | grep "$BUSID" -A20 | grep 'Serial Number' | awk '{print $NF}');`+
						` python3 -c "import os; s=os.environ.get('SERIAL','').replace('-',''); print(int(s,16) if s else 0)"`,
					interfaceName,
				),
				Trim: true,
			},
		},
	)
	if err != nil {
		log.Errorf("failed to create fetcher for dpll clock ID: %s", err.Error())
		return fmt.Errorf("failed to create fetcher for dpll clock ID: %w", err)
	}
	fetcherInst.SetPostProcessor(postProcessDPLLNetlinkClockID)
	dpllClockIDFetcher[interfaceName] = fetcherInst
	return nil
}

func netlinkPinGetCommand(pinID int32) string {
	return fmt.Sprintf(
		"/linux/tools/net/ynl/cli.py --spec /linux/Documentation/netlink/specs/dpll.yaml "+
			`--do pin-get --json '{"id": %d}' 2>/dev/null | python3 /root/custom_scripts/json_encoder.py`,
		pinID,
	)
}

// probeAllNetlinkPins queries each pin id; full pin-get/device-get dumps fail on GNRD
// (ynl spec in dpll-debug:0.5 is older than the host kernel).
func probeAllNetlinkPins(ctx clients.ExecContext) []*NetlinkPin {
	pins := make([]*NetlinkPin, 0)
	command := []string{"/usr/bin/sh"}

	for pinID := int32(0); pinID < maxNetlinkPinProbe; pinID++ {
		var buffIn bytes.Buffer
		buffIn.WriteString(netlinkPinGetCommand(pinID))
		stdout, _, err := ctx.ExecCommandStdIn(command, buffIn)
		if err != nil || strings.TrimSpace(stdout) == "" {
			continue
		}
		pin := &NetlinkPin{}
		if jsonErr := json.Unmarshal([]byte(stdout), pin); jsonErr != nil {
			log.Debugf("skip netlink pin %d: %v", pinID, jsonErr)
			continue
		}
		pins = append(pins, pin)
	}
	return pins
}

func filterPinsByClockID(pins []*NetlinkPin, clockID uint64) []*NetlinkPin {
	matched := make([]*NetlinkPin, 0)
	for _, pin := range pins {
		if pin.ClockID == clockID {
			matched = append(matched, pin)
		}
	}
	return matched
}

func filterIceNetlinkPins(pins []*NetlinkPin) []*NetlinkPin {
	matched := make([]*NetlinkPin, 0)
	for _, pin := range pins {
		if pin.ModuleName == "ice" {
			matched = append(matched, pin)
		}
	}
	return matched
}

// discoverNetlinkPins queries pins individually. A full "pin-get" dump fails on
// GNRD E830 when the kernel exposes attributes newer than the ynl spec in dpll-debug.
func discoverNetlinkPins(ctx clients.ExecContext, clockID uint64) ([]*NetlinkPin, error) {
	allPins := probeAllNetlinkPins(ctx)
	if len(allPins) == 0 {
		return nil, fmt.Errorf("no netlink pins responded (dpll-debug ynl may not match kernel)")
	}

	pins := filterPinsByClockID(allPins, clockID)
	if len(pins) == 0 {
		seen := make([]uint64, 0, len(allPins))
		for _, pin := range allPins {
			seen = append(seen, pin.ClockID)
		}
		log.Warnf(
			"no pins with clock-id %d; probe saw clock-ids %v — using ice driver pins",
			clockID, seen,
		)
		pins = filterIceNetlinkPins(allPins)
	}
	if len(pins) == 0 {
		return pins, fmt.Errorf("no netlink pins found for clock-id %d", clockID)
	}
	return pins, nil
}

func pinParentsConnected(pin *NetlinkPin) bool {
	if pin == nil {
		return false
	}
	for _, parentDev := range pin.ParentDevices {
		if parentDev.State != ConnectedState {
			return false
		}
	}
	return len(pin.ParentDevices) > 0
}

func pinSMA1InputConnected(pin *NetlinkPin) bool {
	if pin == nil {
		return false
	}
	for _, parentDev := range pin.ParentDevices {
		if parentDev.Direction != InputDirection || parentDev.State != ConnectedState {
			return false
		}
	}
	return len(pin.ParentDevices) > 0
}

func selectPin(entries []*NetlinkPin, clockID uint64) (int32, string, error) { //nolint:funlen,gocritic,cyclop // allow slightly longer function for sake of readability
	var OnePPSPin, SMA1Pin *NetlinkPin

	log.Debug("entries: ", entries)
	for _, pin := range entries {
		switch pin.Label {
		case OnePPSLabel:
			OnePPSPin = pin
		case SMA1Label:
			SMA1Pin = pin
		}
	}

	if pinParentsConnected(OnePPSPin) {
		return OnePPSPin.ID, OnePPSLabel, nil
	}
	if pinSMA1InputConnected(SMA1Pin) {
		return SMA1Pin.ID, SMA1Label, nil
	}

	// GNRD E830: ts2phc uses pin_index 1 on each netdev.
	for _, pin := range entries {
		if pin.ID == 1 && pinParentsConnected(pin) {
			label := pin.Label
			if label == "" {
				label = "pin-1"
			}
			return pin.ID, label, nil
		}
	}

	// E830 / GNRD may use different board labels; use any connected pin on this clock.
	for _, pin := range entries {
		if pinParentsConnected(pin) {
			label := pin.Label
			if label == "" {
				label = fmt.Sprintf("pin-%d", pin.ID)
			}
			return pin.ID, label, nil
		}
	}
	return 0, "", errors.New("failed to determin correct offset pin")
}

func parseNetlinkClockID(raw string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty clock id")
	}
	// Bash $((16#...)) returns a signed int64; large PCI serials overflow and appear negative.
	if strings.HasPrefix(raw, "-") {
		signed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse signed clock id: %w", err)
		}
		return uint64(signed), nil
	}
	clockID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse clock id: %w", err)
	}
	return clockID, nil
}

func postProcessDPLLNetlinkClockID(result map[string]string) (map[string]any, error) {
	processedResult := make(map[string]any)
	clockID, err := parseNetlinkClockID(result["dpll-netlink-clock-id"])
	if err != nil {
		return processedResult, err
	}
	processedResult["clockID"] = clockID
	return processedResult, nil
}

type NetlinkParameters struct {
	Timestamp string `fetcherKey:"date"      json:"timestamp"`
	PinType   string `fetcherKey:"pinType"   json:"pinType"`
	ClockID   uint64 `fetcherKey:"clockID"   json:"clockId"`
	OffsetPin int32  `fetcherKey:"offsetPin" json:"offsetPin"`
}

type netlinkClockIDInfo struct {
	Timestamp string `fetcherKey:"date"     json:"timestamp"`
	ClockID   uint64 `fetcherKey:"clockID"  json:"clockId"`
}

func GetNetlinkParameters(ctx clients.ExecContext, interfaceName string) (NetlinkParameters, error) {
	netlinkInfo := NetlinkParameters{}
	fetcherInst, fetchedInstanceOk := dpllClockIDFetcher[interfaceName]
	if !fetchedInstanceOk {
		err := BuildNetlinkInfoFetcher(interfaceName)
		if err != nil {
			return netlinkInfo, err
		}
		fetcherInst, fetchedInstanceOk = dpllClockIDFetcher[interfaceName]
		if !fetchedInstanceOk {
			return netlinkInfo, errors.New("failed to create fetcher for DPLLInfo using netlink interface")
		}
	}
	clockInfo := netlinkClockIDInfo{}
	err := fetcherInst.Fetch(ctx, &clockInfo)
	if err != nil {
		log.Debugf("failed to fetch netlink clock id %s", err.Error())
		return netlinkInfo, fmt.Errorf("failed to fetch netlink clock id %w", err)
	}

	pins, err := discoverNetlinkPins(ctx, clockInfo.ClockID)
	if err != nil {
		return netlinkInfo, err
	}
	offsetPin, pinType, err := selectPin(pins, clockInfo.ClockID)
	if err != nil {
		return netlinkInfo, err
	}

	return NetlinkParameters{
		Timestamp: clockInfo.Timestamp,
		PinType:   pinType,
		ClockID:   clockInfo.ClockID,
		OffsetPin: offsetPin,
	}, nil
}
