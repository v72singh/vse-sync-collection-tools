// SPDX-License-Identifier: GPL-2.0-or-later

package validations

import (
	"fmt"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/devices"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/utils"
)

const (
	deviceDetailsID          = TGMEnvModelPath + "/nic/"
	deviceDetailsDescription = "Verify NIC device model"
)

var (
	VendorIntel        = "0x8086"
	E810WesportChannel = "0x1593"
	E810LoganBeach     = "0x1592"
	E825C              = "0x579e"
	E830               = "0x12d3"
)

type intelNICProfile struct {
	family      string
	minFirmware string
}

// supportedIntelNICs lists Intel NICs used for T-GM / GNRD collection.
var supportedIntelNICs = map[string]intelNICProfile{
	E810WesportChannel: {family: "E810", minFirmware: "4.20"},
	E810LoganBeach:     {family: "E810", minFirmware: "4.20"},
	E825C:              {family: "E825-C", minFirmware: "4.03"},
	E830:               {family: "E830", minFirmware: "1.12"},
}

func isSupportedIntelPTPNIC(deviceID string) bool {
	_, ok := supportedIntelNICs[deviceID]
	return ok
}

type DeviceDetails struct {
	VendorID string `json:"vendorId"`
	DeviceID string `json:"deviceId"`
}

func (dev *DeviceDetails) Verify() error {
	if dev.VendorID != VendorIntel {
		return utils.NewInvalidEnvError(fmt.Errorf("NIC vendor is not Intel (got %s)", dev.VendorID))
	}
	if !isSupportedIntelPTPNIC(dev.DeviceID) {
		return utils.NewInvalidEnvError(fmt.Errorf("NIC device %s is not a supported Intel PTP NIC", dev.DeviceID))
	}
	return nil
}

func (dev *DeviceDetails) GetID() string {
	return deviceDetailsID
}

func (dev *DeviceDetails) GetDescription() string {
	return deviceDetailsDescription
}

func (dev *DeviceDetails) GetData() any { //nolint:ireturn // data will very for each validation
	return dev
}

func (dev *DeviceDetails) GetOrder() int {
	return deviceDetailsOrdering
}

func NewDeviceDetails(ptpDevInfo *devices.PTPDeviceInfo) *DeviceDetails {
	return &DeviceDetails{
		VendorID: ptpDevInfo.VendorID,
		DeviceID: ptpDevInfo.DeviceID,
	}
}
