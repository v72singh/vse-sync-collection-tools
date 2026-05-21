// SPDX-License-Identifier: GPL-2.0-or-later

package validations

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/utils"
)

const (
	TGMTestIDBase   = "https://github.com/redhat-partner-solutions/vse-sync-test/tree/main/tests"
	TGMEnvModelPath = TGMTestIDBase + "/environment/model"
	TGMEnvVerPath   = TGMTestIDBase + "/environment/version"
	TGMSyncEnvPath  = TGMTestIDBase + "/sync/G.8272/environment/status"
)

const (
	clusterVersionOrdering int = iota
	ptpOperatorVersionOrdering
	gpsdVersionOrdering
	deviceDetailsOrdering
	deviceDriverVersionOrdering
	deviceFirmwareOrdering
	gnssModuleOrdering
	gnssVersionOrdering
	gnssProtOrdering
	hasGNSSDevicesOrdering
	gnssConnectedToAntOrdering
	gnssReceivingDataOrdering
	configuredForGrandMasterOrdering
)

type VersionCheck struct {
	id           string `json:"-"`
	Version      string `json:"version"`
	checkVersion string `json:"-"`
	MinVersion   string `json:"expected"`
	description  string `json:"-"`
	order        int    `json:"-"`
}

func nicFirmwareSemver(checkVersion string) (string, error) {
	ver := fmt.Sprintf("v%s", strings.ReplaceAll(checkVersion, "_", "-"))
	if semver.IsValid(ver) {
		return ver, nil
	}
	// ice reports two-part versions such as 4.03; golang semver rejects v4.03 (leading zero).
	parts := strings.Split(checkVersion, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("could not parse version %s", ver)
	}
	minor := strings.TrimLeft(parts[1], "0")
	if minor == "" {
		minor = "0"
	}
	patch := "0"
	if len(parts) > 2 {
		patch = parts[2]
	}
	ver = fmt.Sprintf("v%s.%s.%s", parts[0], minor, patch)
	if !semver.IsValid(ver) {
		return "", fmt.Errorf("could not parse version %s", ver)
	}
	return ver, nil
}

func (verCheck *VersionCheck) Verify() error {
	ver, err := nicFirmwareSemver(verCheck.checkVersion)
	if err != nil {
		return err
	}
	minVer, err := nicFirmwareSemver(verCheck.MinVersion)
	if err != nil {
		minVer = fmt.Sprintf("v%s", verCheck.MinVersion)
		if !semver.IsValid(minVer) {
			return err
		}
	}
	if semver.Compare(ver, minVer) < 0 {
		return utils.NewInvalidEnvError(
			fmt.Errorf("unexpected version: %s < %s", verCheck.checkVersion, verCheck.MinVersion),
		)
	}
	return nil
}

func (verCheck *VersionCheck) GetID() string {
	return verCheck.id
}

func (verCheck *VersionCheck) GetDescription() string {
	return verCheck.description
}

func (verCheck *VersionCheck) GetData() any { //nolint:ireturn // data will vary for each validation
	return verCheck
}

func (verCheck *VersionCheck) GetOrder() int {
	return verCheck.order
}

type VersionWithError struct {
	Error   error  `json:"fetchError"`
	Version string `json:"version"`
}

func MarshalVersionAndError(ver *VersionWithError) ([]byte, error) {
	var err any
	if ver.Error != nil {
		err = ver.Error.Error()
	}
	marsh, marshalErr := json.Marshal(&struct {
		Error   any    `json:"fetchError"`
		Version string `json:"version"`
	}{
		Version: ver.Version,
		Error:   err,
	})
	return marsh, fmt.Errorf("failed to marshal VersionWithError %w", marshalErr)
}

type VersionWithErrorCheck struct {
	Error error
	VersionCheck
}

func (verCheck *VersionWithErrorCheck) MarshalJSON() ([]byte, error) {
	return MarshalVersionAndError(&VersionWithError{
		Version: verCheck.Version,
		Error:   verCheck.Error,
	})
}

func (verCheck *VersionWithErrorCheck) Verify() error {
	if verCheck.Error != nil {
		return verCheck.Error
	}
	return verCheck.VersionCheck.Verify()
}
