// SPDX-License-Identifier: GPL-2.0-or-later
//
// Collector that reports Telecom Profile clock attributes derived from PMC outputs.
package collectors

import (
	"fmt"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/callbacks"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/clients"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/contexts"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/devices"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/utils"
)

const (
	TelecomClockProfileCollectorName = "Telecom-Clock-Profile"
	TelecomClockProfileInfo          = "telecom-clock-profile"
)

type TelecomClockProfileCollector struct {
	*baseCollector

	ctx       clients.ExecContext
	clockType string
}

func telecomClockProfilePoller(c *TelecomClockProfileCollector) func() (callbacks.OutputType, error) {
	return func() (callbacks.OutputType, error) {
		pmc, err := devices.GetPMC(c.ctx)
		if err != nil {
			return nil, err //nolint:wrapcheck // propagate
		}
		return devices.BuildTelecomClockProfileFromPMC(pmc, c.clockType), nil
	}
}

// Poll collects information from the cluster then
// calls the callback.Call to allow that to persist it
func (c *TelecomClockProfileCollector) Poll(resultsChan chan PollResult, wg *utils.WaitGroupCount) {
	defer wg.Done()

	errorsToReturn := make([]error, 0)

	err := c.poll()
	if err != nil {
		errorsToReturn = append(errorsToReturn, err)
	}

	resultsChan <- PollResult{
		CollectorName: TelecomClockProfileCollectorName,
		Errors:        errorsToReturn,
	}
}

// Returns a new TelecomClockProfileCollector based on values in the CollectionConstructor
func NewTelecomClockProfileCollector(constructor *CollectionConstructor) (Collector, error) {
	ctx, err := contexts.GetPTPDaemonContext(constructor.Clientset, constructor.PTPNodeName)
	if err != nil {
		return &TelecomClockProfileCollector{}, fmt.Errorf("failed to create TelecomClockProfileCollector: %w", err)
	}

	collector := &TelecomClockProfileCollector{
		baseCollector: newBaseCollector(
			constructor.PollInterval,
			false,
			constructor.Callback,
			TelecomClockProfileCollectorName,
			TelecomClockProfileInfo,
		),
		ctx:       ctx,
		clockType: constructor.ClockType,
	}
	collector.poller = telecomClockProfilePoller(collector)

	return collector, nil
}

func init() {
	RegisterCollector(TelecomClockProfileCollectorName, NewTelecomClockProfileCollector, optional)
}
