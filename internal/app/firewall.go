package app

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// OpenFirewallPort opens a port in ufw / firewalld if one of them is active.
// It is best-effort: missing tools or a non-root process are silently ignored.
func OpenFirewallPort(port int, proto string) {
	if port <= 0 || port > 65535 || proto == "none" || os.Geteuid() != 0 {
		return
	}
	protos := []string{"tcp"}
	switch proto {
	case "udp":
		protos = []string{"udp"}
	case "both":
		protos = []string{"tcp", "udp"}
	}
	p := strconv.Itoa(port)
	if path, err := exec.LookPath("ufw"); err == nil {
		out, _ := exec.Command(path, "status").CombinedOutput()
		if strings.Contains(string(out), "Status: active") {
			for _, pr := range protos {
				_ = exec.Command(path, "allow", p+"/"+pr).Run()
			}
		}
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		if exec.Command(path, "--state").Run() == nil {
			for _, pr := range protos {
				_ = exec.Command(path, "--permanent", "--add-port="+p+"/"+pr).Run()
				_ = exec.Command(path, "--add-port="+p+"/"+pr).Run()
			}
		}
	}
}

func inboundFirewallProto(in *Inbound) string {
	if in.Listen == "127.0.0.1" || in.Listen == "::1" || in.Listen == "localhost" {
		return "none" // local-only inbound (e.g. behind the decoy site)
	}
	return inboundL4(in)
}
