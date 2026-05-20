// SPDX-License-Identifier: GPL-2.0-or-later

package detect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/clients"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/contexts"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/constants"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/utils"
)

type DetectedInterface struct {
	Name               string `json:"name"`
	PTPClockDevicePath string `json:"ptp_dev"` //nolint:tagliatelle // script assumes
	Primary            bool   `json:"primary"`
}

// sortAndDeduplicateInterfaces sorts interfaces by Primary (primary first) then by Name (alphabetically),
// and deduplicates based on PTP device path, keeping the first occurrence
func sortAndDeduplicateInterfaces(interfaces []DetectedInterface) []DetectedInterface {
	if len(interfaces) == 0 {
		return interfaces
	}

	// First, deduplicate based on PTP device path
	// Use a map to track seen PTP devices and keep only the first occurrence
	seen := make(map[string]bool)
	deduplicated := make([]DetectedInterface, 0, len(interfaces))

	// Sort by Primary (primary first) then by Name (alphabetically)
	sort.Slice(interfaces, func(i, j int) bool {
		// Primary interfaces come first
		if interfaces[i].Primary != interfaces[j].Primary {
			return interfaces[i].Primary // true comes before false
		}
		// If Primary status is the same, sort by Name alphabetically
		return interfaces[i].Name < interfaces[j].Name
	})

	for _, iface := range interfaces {
		key := iface.PTPClockDevicePath
		if key == "" {
			key = iface.Name
		}
		if !seen[key] {
			seen[key] = true
			deduplicated = append(deduplicated, iface)
		} else {
			log.Infof("Deduplicating interface %s with PTP device %s (already seen)",
				iface.Name, iface.PTPClockDevicePath)
		}
	}

	return deduplicated
}

func Detect(kubeConfig, ptpNodeName string, outputAsJSON bool, clockType string) {
	clientset, err := clients.GetClientset(kubeConfig)
	utils.IfErrorExitOrPanic(err)
	ctx, err := contexts.GetPTPDaemonContext(clientset, ptpNodeName)
	utils.IfErrorExitOrPanic(err)
	interfaces, err := checkPTPConfig(ctx, clockType)
	utils.IfErrorExitOrPanic(err)
	output(os.Stdout, interfaces, outputAsJSON)
}

func output(outWriter io.Writer, interfaces []DetectedInterface, outputAsJSON bool) {
	if outputAsJSON {
		out, err := json.MarshalIndent(interfaces, "", "  ")
		utils.IfErrorExitOrPanic(err)
		_, err = outWriter.Write(out)
		utils.IfErrorExitOrPanic(err)
	} else {
		_, err := fmt.Fprintf(outWriter, "%T(%v)", interfaces, interfaces)
		utils.IfErrorExitOrPanic(err)
	}
}

func parseConfig(contents string) (map[string][]string, error) {
	scanner := bufio.NewScanner(strings.NewReader(contents))

	var currentSection string

	result := make(map[string][]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			continue
		} else if currentSection != "" {
			result[currentSection] = append(result[currentSection], line)
		}
	}

	err := scanner.Err()
	if err != nil {
		return result, fmt.Errorf("failed when parsing config: %w", err)
	}

	return result, nil
}

var (
	ts2phcNotMaster = regexp.MustCompile(`ts2phc.master\s+0`)
	ts2phcMaster    = regexp.MustCompile(`ts2phc.master\s+1`)
	ptp4lMasterOnly = regexp.MustCompile(`masterOnly\s+1`)
	ptp4lServerOnly = regexp.MustCompile(`serverOnly\s+1`)
	ptpDevicePath   = regexp.MustCompile(`^/dev/ptp[0-9]+$`)
)

var errNoPTPClockDevice = errors.New("no PTP clock device found")

func isSkippedConfigSection(section string) bool {
	switch strings.ToLower(strings.TrimSpace(section)) {
	case "global", "nmea":
		return true
	default:
		return false
	}
}

func resolvePTPClockDevicePath(section string) string {
	section = strings.TrimSpace(section)
	if ptpDevicePath.MatchString(section) {
		return section
	}
	return ""
}

func isValidPTPClockIndex(clockNumber string) bool {
	if clockNumber == "" || clockNumber == "none" || clockNumber == "unknown" {
		return false
	}
	for _, c := range clockNumber {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func ptpClockFromEthtool(output string) (string, error) {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "PTP Hardware Clock:") {
			clockNumber := strings.TrimSpace(strings.Split(line, ":")[1])
			if isValidPTPClockIndex(clockNumber) {
				return "/dev/ptp" + clockNumber, nil
			}
		}
	}
	return "", errNoPTPClockDevice
}

func getPTPClockDeviceFromSysfs(ctx clients.ExecContext, interfaceName string) (string, error) {
	out, _, err := ctx.ExecCommand([]string{"readlink", "-f", "/sys/class/net/" + interfaceName + "/device/ptp"})
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(out)
	if base == "" {
		return "", errNoPTPClockDevice
	}
	ptpName := base[strings.LastIndex(base, "/")+1:]
	if !strings.HasPrefix(ptpName, "ptp") {
		return "", errNoPTPClockDevice
	}
	return "/dev/" + ptpName, nil
}

func getPTPClockDevice(ctx clients.ExecContext, interfaceName string) (string, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if ptpDev := resolvePTPClockDevicePath(interfaceName); ptpDev != "" {
		return ptpDev, nil
	}

	out, _, err := ctx.ExecCommand([]string{"ethtool", "-T", interfaceName})
	if err == nil {
		ptpDev, err := ptpClockFromEthtool(out)
		if err == nil {
			return ptpDev, nil
		}
	}

	ptpDev, sysfsErr := getPTPClockDeviceFromSysfs(ctx, interfaceName)
	if sysfsErr == nil {
		return ptpDev, nil
	}

	if err != nil {
		return "", fmt.Errorf("failed to get ptp clock number for %q: %w", interfaceName, err)
	}
	return "", fmt.Errorf("%w for %q", errNoPTPClockDevice, interfaceName)
}

func netdevExists(ctx clients.ExecContext, interfaceName string) bool {
	_, _, err := ctx.ExecCommand([]string{"test", "-e", "/sys/class/net/" + interfaceName})
	return err == nil
}

func hasGNSSDevice(ctx clients.ExecContext, interfaceName string) bool {
	_, _, err := ctx.ExecCommand([]string{"test", "-e", "/sys/class/net/" + interfaceName + "/device/gnss"})
	return err == nil
}

func sectionHasTs2phcMaster(lines []string) bool {
	for _, l := range lines {
		if ts2phcMaster.MatchString(l) {
			return true
		}
	}
	return false
}

func nmeaSectionIsMaster(config map[string][]string) bool {
	lines, ok := config["nmea"]
	if !ok {
		return false
	}
	return sectionHasTs2phcMaster(lines)
}

// applyNmeaMasterPrimary handles GNRD-style profiles where ts2phc.master 1 is on
// [nmea] and all netdev sections are ts2phc.master 0 (extts/DPLL slaves).
func applyNmeaMasterPrimary(ctx clients.ExecContext, detected []DetectedInterface, config map[string][]string) []DetectedInterface {
	if !nmeaSectionIsMaster(config) || len(detected) == 0 {
		return detected
	}

	primaryIface := ""
	if ctx != nil {
		for _, iface := range detected {
			if !hasGNSSDevice(ctx, iface.Name) {
				continue
			}
			if primaryIface == "" || iface.Name < primaryIface {
				primaryIface = iface.Name
			}
		}
	}

	if primaryIface == "" {
		for _, iface := range detected {
			if iface.PTPClockDevicePath == "" {
				continue
			}
			if primaryIface == "" || iface.Name < primaryIface {
				primaryIface = iface.Name
			}
		}
	}

	if primaryIface == "" {
		primaryIface = detected[0].Name
		for _, iface := range detected[1:] {
			if iface.Name < primaryIface {
				primaryIface = iface.Name
			}
		}
	}

	for i := range detected {
		detected[i].Primary = detected[i].Name == primaryIface
	}

	log.Infof(
		"ts2phc master is [nmea]; marking %q as primary (GNSS netdev for GM collection)",
		primaryIface,
	)

	return detected
}

func appendDetectedInterface(
	detected []DetectedInterface,
	ctx clients.ExecContext,
	section string,
	lines []string,
	isPrimary func([]string) bool,
) []DetectedInterface {
	section = strings.TrimSpace(section)
	if isSkippedConfigSection(section) {
		return detected
	}

	primary := isPrimary(lines)
	ptpDev, err := getPTPClockDevice(ctx, section)
	if err != nil {
		if !primary && netdevExists(ctx, section) {
			log.Warnf(
				"section %q has no PHC via ethtool/sysfs; including for DPLL collection only",
				section,
			)
			return append(detected, DetectedInterface{
				Name:    section,
				Primary: false,
			})
		}
		log.Warnf("skipping section %q: %v", section, err)
		return detected
	}

	return append(detected, DetectedInterface{
		Name:               section,
		Primary:            primary,
		PTPClockDevicePath: ptpDev,
	})
}

func getDetectedInterfaces(ctx clients.ExecContext, config map[string][]string) []DetectedInterface {
	detected := []DetectedInterface{}
	nmeaMaster := nmeaSectionIsMaster(config)

	for section, lines := range config {
		detected = appendDetectedInterface(detected, ctx, section, lines, func(ls []string) bool {
			if nmeaMaster {
				// netdev sections are extts slaves; primary is chosen after detection
				return false
			}
			for _, l := range ls {
				if ts2phcNotMaster.MatchString(l) {
					return false
				}
			}
			return true
		})
	}

	detected = applyNmeaMasterPrimary(ctx, detected, config)

	return sortAndDeduplicateInterfaces(detected)
}

func checkPTPConfig(ctx clients.ExecContext, clockType string) ([]DetectedInterface, error) {
	if clockType == constants.ClockTypeBC {
		// For BC clocks, try ptp4l config first
		interfaces, err := checkPtp4lConfig(ctx)
		if err != nil {
			log.Info("ptp4l config not found, falling back to ts2phc config for BC clock")
			return checkTs2PhcConfig(ctx)
		}

		return interfaces, nil
	} else {
		// For GM clocks, try ts2phc config first
		interfaces, err := checkTs2PhcConfig(ctx)
		if err != nil {
			log.Info("ts2phc config not found, falling back to ptp4l config for GM clock")
			return checkPtp4lConfig(ctx)
		}

		return interfaces, nil
	}
}

func checkPtp4lConfig(ctx clients.ExecContext) ([]DetectedInterface, error) {
	errs := []error{}
	detected := []DetectedInterface{}

	files, _, err := ctx.ExecCommand([]string{"ls", "/var/run/"})
	if err != nil {
		return nil, fmt.Errorf("failed to list /var/run/ directory: %w", err)
	}

	ptp4lConfigFiles := make([]string, 0)

	for f := range strings.FieldsSeq(files) {
		if strings.HasPrefix(f, "ptp4l.") && strings.HasSuffix(f, ".config") {
			ptp4lConfigFiles = append(ptp4lConfigFiles, f)
		}
	}

	if len(ptp4lConfigFiles) == 0 {
		return nil, errors.New("failed to find ptp4l config file")
	} else if len(ptp4lConfigFiles) > 1 {
		log.Warnf("Multiple ptp4l profiles found (%v)", ptp4lConfigFiles)
	}

	for _, ptp4lConfigPath := range ptp4lConfigFiles {
		ptp4lConfig, _, err := ctx.ExecCommand([]string{"cat", "/var/run/" + ptp4lConfigPath})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read ptp4l config file: %w", err))
			continue
		}

		config, err := parseConfig(ptp4lConfig)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse ptp4l config file: %w", err))
			continue
		}

		detected = append(detected, getDetectedInterfacesFromPtp4l(ctx, config)...)
	}

	return detected, utils.MakeCompositeError("", errs) //nolint:wrapcheck //this just combines errors.
}

func getDetectedInterfacesFromPtp4l(ctx clients.ExecContext, config map[string][]string) []DetectedInterface {
	detected := []DetectedInterface{}

	for section, lines := range config {
		detected = appendDetectedInterface(detected, ctx, section, lines, func(ls []string) bool {
			for _, l := range ls {
				if ptp4lMasterOnly.MatchString(l) || ptp4lServerOnly.MatchString(l) {
					return false
				}
			}
			return true
		})
	}

	return sortAndDeduplicateInterfaces(detected)
}

func checkTs2PhcConfig(ctx clients.ExecContext) ([]DetectedInterface, error) { //nolint:staticcheck //Suggestion looks bad
	errs := []error{}
	detected := []DetectedInterface{}

	files, _, err := ctx.ExecCommand([]string{"ls", "/var/run/"})
	if err != nil {
		return nil, fmt.Errorf("failed to list /var/run/ directory: %w", err)
	}

	ts2phcConfigFiles := make([]string, 0)

	for f := range strings.FieldsSeq(files) {
		if strings.HasPrefix(f, "ts2phc.") && strings.HasSuffix(f, ".config") {
			ts2phcConfigFiles = append(ts2phcConfigFiles, f)
		}
	}

	if len(ts2phcConfigFiles) == 0 {
		return nil, errors.New("failed to find ts2phc config file")
	} else if len(ts2phcConfigFiles) > 1 {
		log.Warnf("Multiple profiles found (%v)", ts2phcConfigFiles)
	}

	for _, ts2phcConfigPath := range ts2phcConfigFiles {
		ts2phcConfig, _, err := ctx.ExecCommand([]string{"cat", "/var/run/" + ts2phcConfigPath})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read ts2 config file: %w", err))
		}

		config, err := parseConfig(ts2phcConfig)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse ts2 config file: %w", err))
		}

		detected = append(detected, getDetectedInterfaces(ctx, config)...)
	}

	detected = sortAndDeduplicateInterfaces(detected)
	if len(detected) == 0 {
		errs = append(errs, errors.New("no PTP-capable interfaces detected from ts2phc config"))
	} else if !hasPrimaryInterface(detected) {
		errs = append(errs, errors.New("no primary interface detected from ts2phc config"))
	}

	return detected, utils.MakeCompositeError("", errs) //nolint:wrapcheck //this just combines errors.
}

func hasPrimaryInterface(detected []DetectedInterface) bool {
	for _, iface := range detected {
		if iface.Primary {
			return true
		}
	}
	return false
}
