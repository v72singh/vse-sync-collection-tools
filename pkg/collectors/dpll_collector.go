// SPDX-License-Identifier: GPL-2.0-or-later

package collectors

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/contexts"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/devices"
)

const (
	DPLLCollectorName = "DPLL"
)

// Returns a new DPLLCollector from the CollectionConstuctor Factory
func NewDPLLCollector(constructor *CollectionConstructor) (Collector, error) {
	ptpCtx, err := contexts.GetPTPDaemonContext(constructor.Clientset, constructor.PTPNodeName)
	if err != nil {
		return &DPLLNetlinkCollector{}, fmt.Errorf("failed to create DPLLCollector: %w", err)
	}
	dpllFSExists, fsErr := devices.IsDPLLFileSystemPresent(ptpCtx, constructor.PTPInterface)
	log.Debug("DPLL FS exists (linuxptp pod): ", dpllFSExists)
	if dpllFSExists && fsErr == nil {
		return NewDPLLFilesystemCollector(constructor)
	}

	netlinkCtx, err := contexts.GetNetlinkContext(
		constructor.Clientset,
		constructor.PTPNodeName,
		constructor.UnmanagedDebugPod,
	)
	if err != nil {
		return &DPLLNetlinkCollector{}, fmt.Errorf("failed to create DPLLCollector: %w", err)
	}
	// e2e.sh runs start-debug before collect; the debug pod can see host sysfs.
	dpllFSOnHost, hostErr := devices.IsDPLLFileSystemPresent(netlinkCtx, constructor.PTPInterface)
	log.Debug("DPLL FS exists (debug pod / host sysfs): ", dpllFSOnHost)
	if dpllFSOnHost && hostErr == nil {
		return NewDPLLFilesystemCollectorHost(constructor, netlinkCtx)
	}

	return NewDPLLNetlinkCollector(constructor)
}

func init() {
	RegisterCollector(DPLLCollectorName, NewDPLLCollector, optional)
}
