package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
)

const (
	defaultMTU         = 1420
	defaultRouteMetric = 100
)

type checkResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type verifyReport struct {
	GeneratedAt  string                   `json:"generated_at"`
	Platform     string                   `json:"platform"`
	Passed       bool                     `json:"passed"`
	Capabilities p2p.PlatformCapabilities `json:"capabilities"`
	Checks       []checkResult            `json:"checks"`
	State        virtualnet.State         `json:"state"`
}

func main() {
	report := runVerification()
	writeReportAndExit(report)
}

func runVerification() (report verifyReport) {
	var interfaceName string
	var virtualIP string
	var peerIP string
	var subnet string
	var gateway string
	var routeDestination string
	var mtu int
	var skipRouteApply bool

	flag.StringVar(&interfaceName, "interface", "nextunnel0", "TUN interface name")
	flag.StringVar(&virtualIP, "virtual-ip", "10.7.0.10", "local virtual IP")
	flag.StringVar(&peerIP, "peer-ip", "10.7.0.1", "peer virtual IP")
	flag.StringVar(&subnet, "subnet", "10.7.0.0/24", "virtual subnet")
	flag.StringVar(&gateway, "gateway", "10.7.0.1", "virtual gateway")
	flag.StringVar(&routeDestination, "route", "10.7.0.0/24", "route destination to apply")
	flag.IntVar(&mtu, "mtu", defaultMTU, "TUN MTU")
	flag.BoolVar(&skipRouteApply, "skip-route-apply", false, "only verify real TUN creation")
	flag.Parse()

	report = verifyReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Platform:     runtime.GOOS,
		Capabilities: p2p.CurrentPlatform(),
		Checks:       make([]checkResult, 0, 6),
	}

	_, parsedSubnet, err := net.ParseCIDR(subnet)
	if err != nil {
		report.add("parse_subnet", false, err.Error())
		return report
	}
	report.add("parse_subnet", true, subnet)

	report.add("tun_preflight", report.Capabilities.KernelTUNReady, platformCapabilityDetail(report.Capabilities))

	tunConfig := p2p.TUNConfig{
		Name:    interfaceName,
		MTU:     mtu,
		LocalIP: net.ParseIP(virtualIP),
		PeerIP:  net.ParseIP(peerIP),
		Subnet:  parsedSubnet,
	}
	device, helperBacked, err := createVerificationTUN(tunConfig)
	if err != nil {
		report.add("create_tun", false, err.Error())
		return report
	}
	defer device.Close()

	name, _ := device.Name()
	report.add("create_tun", name != "netTun", fmt.Sprintf("name=%s", name))
	if name == "netTun" {
		return report
	}

	if skipRouteApply {
		report.add("route_apply_skipped", true, "skip-route-apply=true")
		return report
	}

	manager := newVerificationVirtualNetworkManager(helperBacked)
	state, err := manager.Apply(virtualnet.Config{
		NodeID:    "tun-verify",
		VirtualIP: virtualIP,
		Subnet:    subnet,
		Gateway:   gateway,
		Interface: name,
		MTU:       mtu,
		Routes: []virtualnet.Route{
			{
				Destination: routeDestination,
				Gateway:     gateway,
				Interface:   name,
				Metric:      defaultRouteMetric,
			},
		},
	})
	report.State = state
	if err != nil {
		report.add("route_apply", false, err.Error())
		return report
	}
	report.add("route_apply", state.Applied, fmt.Sprintf("commands=%d", len(state.LastCommands)))

	resetState, err := manager.Reset()
	report.State = resetState
	if err != nil {
		report.add("route_reset", false, err.Error())
		return report
	}
	report.add("route_reset", !resetState.Applied, fmt.Sprintf("commands=%d", len(resetState.LastCommands)))

	return report
}

func (r *verifyReport) add(name string, passed bool, detail string) {
	r.Checks = append(r.Checks, checkResult{Name: name, Passed: passed, Detail: detail})
}

func platformCapabilityDetail(capabilities p2p.PlatformCapabilities) string {
	if len(capabilities.BlockingIssues) == 0 {
		return "kernel_tun_ready=true"
	}
	codes := make([]string, 0, len(capabilities.BlockingIssues))
	for _, issue := range capabilities.BlockingIssues {
		codes = append(codes, issue.Code)
	}
	return "blocking=" + strings.Join(codes, ",")
}

func (r *verifyReport) finalize() {
	r.Passed = true
	for _, check := range r.Checks {
		if !check.Passed {
			r.Passed = false
			return
		}
	}
}

func writeReportAndExit(report verifyReport) {
	report.finalize()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(report)
	if !report.Passed {
		os.Exit(1)
	}
}
