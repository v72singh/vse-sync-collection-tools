// SPDX-License-Identifier: GPL-2.0-or-later

package validations

import (
	"strings"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/devices"
)

const (
	deviceFirmwareID          = TGMEnvVerPath + "/nic-firmware/"
	deviceFirmwareDescription = "Verify NIC firmware version"
)

var (
	MinFirmwareVersion = "4.20"
)

func NewDeviceFirmware(ptpDevInfo *devices.PTPDeviceInfo) *VersionCheck {
	parts := strings.Split(ptpDevInfo.FirmwareVersion, " ")
	minVer := MinFirmwareVersion
	if prof, ok := supportedIntelNICs[ptpDevInfo.DeviceID]; ok {
		minVer = prof.minFirmware
	}
	return &VersionCheck{
		id:           deviceFirmwareID,
		Version:      ptpDevInfo.FirmwareVersion,
		checkVersion: parts[0],
		MinVersion:   minVer,
		description:  deviceFirmwareDescription,
		order:        deviceFirmwareOrdering,
	}
}
