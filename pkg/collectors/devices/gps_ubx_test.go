// SPDX-License-Identifier: GPL-2.0-or-later

package devices_test

import (
	"bufio"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/tools/remotecommand"

	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/clients"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/pkg/collectors/devices"
	"github.com/redhat-partner-solutions/vse-sync-collection-tools/testutils"
)

var _ = Describe("GetGPSNav", func() {
	var clientset *clients.Clientset
	var response map[string][]byte
	BeforeEach(func() { //nolint:dupl // this is test setup code
		clientset = testutils.GetMockedClientSet(testPod)
		response = make(map[string][]byte)
		responder := func(method string, url *url.URL, options remotecommand.StreamOptions) ([]byte, []byte, error) {
			reader := bufio.NewReader(options.Stdin)
			cmd := ""
			keepReading := true
			for keepReading {
				line, prefix, _ := reader.ReadLine()
				keepReading = prefix
				cmd += string(line)
			}
			return response[cmd], []byte(""), nil
		}
		clients.NewSPDYExecutor = testutils.NewFakeNewSPDYExecutor(responder, nil)
	})

	When("called GetGPSNav", func() {
		It("should return a valid GPSNav", func() {
			expectedInput := "echo '<GPS>';ubxtool -t -p NAV-STATUS -p NAV-CLOCK -p MON-RF -P 29.20;echo '</GPS>';"

			expectedOutput := strings.Join([]string{
				"<GPS>",
				"1686916187.0584",
				"UBX-MON-RF:",
				" version 0 nBlocks 2 reserved1 0 0",
				"   blockId 0 flags x0 antStatus 2 antPower 1 postStatus 0 reserved2 0 0 0 0",
				"    noisePerMS 82 agcCnt 6318 jamInd 3 ofsI 15 magI 154 ofsQ 2 magQ 145",
				"    reserved3 0 0 0",
				"   blockId 1 flags x0 antStatus 2 antPower 1 postStatus 0 reserved2 0 0 0 0",
				"    noisePerMS 49 agcCnt 6669 jamInd 2 ofsI -11 magI 146 ofsQ -1 magQ 139",
				"    reserved3 0 0 0",
				"",
				"1686916187.0584",
				"UBX-NAV-STATUS:",
				"  iTOW 474605000 gpsFix 3 flags 0xdd fixStat 0x0 flags2 0x8",
				"  ttff 25030, msss 4294967295",
				"",
				"1686916187.0586",
				"UBX-NAV-CLOCK:",
				"  iTOW 474605000 clkB -61594 clkD -56 tAcc 5 fAcc 164",
				"</GPS>",
			}, "\n")
			response[expectedInput] = []byte(expectedOutput)

			ctx, err := clients.NewContainerContext(clientset, "TestNamespace", "Test", "TestContainer", "TestNodeName")
			Expect(err).NotTo(HaveOccurred())

			gpsInfo, err := devices.GetGPSNav(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(gpsInfo.NavStatus.Timestamp).To(Equal("2023-06-16T11:49:47.0584Z"))
			Expect(gpsInfo.NavStatus.GPSFix).To(Equal(3))

			Expect(gpsInfo.NavClock.Timestamp).To(Equal("2023-06-16T11:49:47.0586Z"))
			Expect(gpsInfo.NavClock.TimeAcc).To(Equal(5))
			Expect(gpsInfo.NavClock.FreqAcc).To(Equal(164))

			Expect(gpsInfo.AntennaDetails[0].Timestamp).To(Equal("2023-06-16T11:49:47.0584Z"))
			Expect(gpsInfo.AntennaDetails[0].BlockID).To(Equal(0))
			Expect(gpsInfo.AntennaDetails[0].Status).To(Equal(2))
			Expect(gpsInfo.AntennaDetails[0].Power).To(Equal(1))

			Expect(gpsInfo.AntennaDetails[1].Timestamp).To(Equal("2023-06-16T11:49:47.0584Z"))
			Expect(gpsInfo.AntennaDetails[1].BlockID).To(Equal(1))
			Expect(gpsInfo.AntennaDetails[1].Status).To(Equal(2))
			Expect(gpsInfo.AntennaDetails[1].Power).To(Equal(1))

		})

		It("should parse GNRD-style UBX-MON-RF blocks", func() {
			expectedInput := "echo '<GPS>';ubxtool -t -p NAV-STATUS -p NAV-CLOCK -p MON-RF -P 29.20;echo '</GPS>';"

			expectedOutput := strings.Join([]string{
				"<GPS>",
				"1779339987.3822",
				"UBX-MON-RF:",
				" version 0 nBlocks 2 reserved1 x0",
				"   0: blockId 0 flags x0 antStatus 2 antPower 1 postStatus 0 reserved2 x0",
				"      noisePerMS 55 agcCnt 5265 jamInd 60 ofsI 13 magI 147 ofsQ 0 magQ 145",
				"      reserved3 0 0 0",
				"   1: blockId 1 flags x0 antStatus 2 antPower 1 postStatus 0 reserved2 x0",
				"      noisePerMS 43 agcCnt 5616 jamInd 41 ofsI 9 magI 167 ofsQ 3 magQ 167",
				"      reserved3 0 0 0",
				"",
				"1779339988.0711",
				"UBX-NAV-STATUS:",
				"  iTOW 364006000 gpsFix 3 flags 0xdd fixStat 0x0 flags2 0x8",
				"  ttff 2086773449, msss 3850696700",
				"",
				"1779339988.0712",
				"UBX-NAV-CLOCK:",
				"  iTOW 364006000 clkB -23824 clkD -152 tAcc 2 fAcc 122",
				"</GPS>",
			}, "\n")
			response[expectedInput] = []byte(expectedOutput)

			ctx, err := clients.NewContainerContext(clientset, "TestNamespace", "Test", "TestContainer", "TestNodeName")
			Expect(err).NotTo(HaveOccurred())

			gpsInfo, err := devices.GetGPSNav(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(gpsInfo.NavStatus.GPSFix).To(Equal(3))
			Expect(len(gpsInfo.AntennaDetails)).To(Equal(2))
		})
	})
})
