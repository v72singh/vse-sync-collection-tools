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
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/utils"
)

type DetectedInterface struct {
	Name               string `json:"name"`
	PTPClockDevicePath string `json:"ptp_dev"` //nolint:tagliatelle // script assumes
	Primary            bool   `json:"primary"`
}

// DetectVersion is logged at startup so runs can confirm the image includes GNRD detect fixes.
const DetectVersion = "20260520-gnrd-nmea-master"

func sortAndDeduplicateInterfaces(interfaces []DetectedInterface) []DetectedInterface {
	if len(interfaces) == 0 {
		return interfaces
	}

	seen := make(map[string]bool)
	deduplicated := make([]DetectedInterface, 0, len(interfaces))

	sort.Slice(interfaces, func(i, j int) bool {
		if interfaces[i].Primary != interfaces[j].Primary {
			return interfaces[i].Primary
		}
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

func exitOnDetectError(err error) {
	if err == nil {
		return
	}
	log.Error(err)
	os.Exit(1)
}

func Detect(kubeConfig, ptpNodeName string, outputAsJSON bool) {
	log.Infof("detect %s", DetectVersion)
	clientset, err := clients.GetClientset(kubeConfig)
	exitOnDetectError(err)
	ctx, err := contexts.GetPTPDaemonContext(clientset, ptpNodeName)
	exitOnDetectError(err)
	interfaces, err := checkTs2PhcConfig(ctx)
	exitOnDetectError(err)
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

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("failed when parsing ts2phc config: %w", err)
	}
	return result, nil
}

var (
	ts2phcNotMaster = regexp.MustCompile(`ts2phc.master\s+0`)
	ts2phcMaster    = regexp.MustCompile(`ts2phc.master\s+1`)
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
	for _, line := range strings.Split(output, "\n") {
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
	script := `iface=` + interfaceName + `
for candidate in /sys/class/net/${iface}/device/ptp /sys/class/net/${iface}/device/ptp/ptp*; do
  if [ -e "$candidate" ]; then
    name=$(basename "$(readlink -f "$candidate")")
    case "$name" in ptp*) echo "/dev/$name"; exit 0 ;; esac
  fi
done
for ptpdir in /sys/class/ptp/ptp*/; do
  if [ -e "${ptpdir}device/net/${iface}" ]; then
    echo "/dev/$(basename "$ptpdir")"
    exit 0
  fi
done
exit 1`
	out, _, err := ctx.ExecCommand([]string{"sh", "-c", script})
	if err != nil {
		return "", err
	}
	ptpDev := strings.TrimSpace(out)
	if ptpDev == "" || !ptpDevicePath.MatchString(ptpDev) {
		return "", errNoPTPClockDevice
	}
	return ptpDev, nil
}

func getPTPClockDevice(ctx clients.ExecContext, interfaceName string) (string, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if ptpDev := resolvePTPClockDevicePath(interfaceName); ptpDev != "" {
		return ptpDev, nil
	}

	out, _, err := ctx.ExecCommand([]string{"ethtool", "-T", interfaceName})
	if err == nil {
		ptpDev, parseErr := ptpClockFromEthtool(out)
		if parseErr == nil {
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
	nmeaMaster bool,
) []DetectedInterface {
	section = strings.TrimSpace(section)
	if isSkippedConfigSection(section) {
		return detected
	}

	primary := isPrimary(lines)
	ptpDev, err := getPTPClockDevice(ctx, section)
	if err != nil {
		if nmeaMaster || (!primary && netdevExists(ctx, section)) {
			log.Warnf("section %q: %v; including netdev from ts2phc config", section, err)
			return append(detected, DetectedInterface{
				Name:    section,
				Primary: primary,
			})
		}
		log.Warnf("skipping ts2phc section %q: %v", section, err)
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
				return false
			}
			for _, l := range ls {
				if ts2phcNotMaster.MatchString(l) {
					return false
				}
			}
			return true
		}, nmeaMaster)
	}

	return applyNmeaMasterPrimary(ctx, detected, config)
}

func hasPrimaryInterface(detected []DetectedInterface) bool {
	for _, iface := range detected {
		if iface.Primary {
			return true
		}
	}
	return false
}

func checkTs2PhcConfig(ctx clients.ExecContext) ([]DetectedInterface, error) { //nolint:stylecheck //Suggestion looks bad
	errs := []error{}
	detected := []DetectedInterface{}
	files, _, err := ctx.ExecCommand([]string{"ls", "/var/run/"})
	if err != nil {
		return nil, fmt.Errorf("failed to list /var/run/ directory: %w", err)
	}

	ts2phcConfigFiles := make([]string, 0)
	for _, f := range strings.Fields(files) {
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
