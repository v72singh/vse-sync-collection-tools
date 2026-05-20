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

func Detect(kubeConfig, ptpNodeName string, outputAsJSON bool) {
	clientset, err := clients.GetClientset(kubeConfig)
	utils.IfErrorExitOrPanic(err)
	ctx, err := contexts.GetPTPDaemonContext(clientset, ptpNodeName)
	utils.IfErrorExitOrPanic(err)
	interfaces, err := checkTs2PhcConfig(ctx)
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

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("failed when parsing ts2phc config: %w", err)
	}
	return result, nil
}

var (
	notMaster     = regexp.MustCompile(`ts2phc.master\s+0`)
	ptpDevicePath = regexp.MustCompile(`^/dev/ptp[0-9]+$`)
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

func ptpClockFromEthtool(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "PTP Hardware Clock:") {
			clockNumber := strings.TrimSpace(strings.Split(line, ":")[1])
			return fmt.Sprintf("/dev/ptp%s", clockNumber), nil
		}
	}
	return "", errNoPTPClockDevice
}

func getPTPClockDevice(ctx clients.ExecContext, interfaceName string) (string, error) {
	if ptpDev := resolvePTPClockDevicePath(interfaceName); ptpDev != "" {
		return ptpDev, nil
	}

	out, _, err := ctx.ExecCommand([]string{"ethtool", "-T", interfaceName})
	if err != nil {
		return "", fmt.Errorf("failed to get ptp clock number for %q: %w", interfaceName, err)
	}
	ptpDev, err := ptpClockFromEthtool(out)
	if err != nil {
		return "", fmt.Errorf("%w for %q", err, interfaceName)
	}
	return ptpDev, nil
}

func netdevExists(ctx clients.ExecContext, interfaceName string) bool {
	_, _, err := ctx.ExecCommand([]string{"test", "-e", "/sys/class/net/" + interfaceName})
	return err == nil
}

func getDetectedInterfaces(ctx clients.ExecContext, config map[string][]string) []DetectedInterface {
	detected := []DetectedInterface{}
	for section, lines := range config {
		if isSkippedConfigSection(section) {
			continue
		}

		section = strings.TrimSpace(section)
		isPrimary := true
		for _, l := range lines {
			if notMaster.MatchString(l) {
				isPrimary = false
			}
		}

		ptpDev, err := getPTPClockDevice(ctx, section)
		if err != nil {
			if !isPrimary && netdevExists(ctx, section) {
				log.Warnf(
					"section %q has no PHC via ethtool; including for DPLL collection only",
					section,
				)
				detected = append(detected, DetectedInterface{
					Name:    section,
					Primary: false,
				})
				continue
			}
			log.Warnf("skipping ts2phc section %q: %v", section, err)
			continue
		}

		detected = append(detected, DetectedInterface{
			Name:               section,
			Primary:            isPrimary,
			PTPClockDevicePath: ptpDev,
		})
	}
	return detected
}

func checkTs2PhcConfig(ctx clients.ExecContext) ([]DetectedInterface, error) { //nolint:stylecheck //Suggestion looks bad
	errs := []error{}
	detected := []DetectedInterface{}
	files, _, err := ctx.ExecCommand([]string{"ls", "/var/run/"})
	utils.IfErrorExitOrPanic(err)

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
	if len(detected) == 0 {
		errs = append(errs, errors.New("no PTP-capable interfaces detected from ts2phc config"))
	}
	return detected, utils.MakeCompositeError("", errs) //nolint:wrapcheck //this just combines errors.
}
