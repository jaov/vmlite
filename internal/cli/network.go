package cli

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
)

func resolveNetworkMAC(cfg *Config) error {
	if strings.TrimSpace(cfg.Network.MAC) != "" {
		hw, err := net.ParseMAC(cfg.Network.MAC)
		if err != nil {
			return fmt.Errorf("parse network.mac %q: %w", cfg.Network.MAC, err)
		}
		cfg.NetworkMAC = hw.String()
		return nil
	}
	if strings.TrimSpace(cfg.Network.IP) == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("vmlite/" + cfg.Name))
	cfg.NetworkMAC = fmt.Sprintf("52:54:00:%02x:%02x:%02x", sum[0], sum[1], sum[2])
	return nil
}

func dhcpHostXML(mac, ip string) string {
	return fmt.Sprintf("<host mac='%s' ip='%s'/>", mac, ip)
}

func ensureDHCPReservation(cfg *Config, uri string, dryRun bool) error {
	if strings.TrimSpace(cfg.Network.IP) == "" {
		return nil
	}
	if uri == sessionURI {
		return fmt.Errorf("network.ip requires qemu:///system with the default network")
	}
	if cfg.NetworkMAC == "" {
		return fmt.Errorf("network MAC is not resolved")
	}
	if dryRun {
		fmt.Printf("Would reserve IP %s for VM %q (%s)\n", cfg.Network.IP, cfg.Name, cfg.NetworkMAC)
		return nil
	}

	_ = removeDHCPReservation(uri, "default", cfg.NetworkMAC, cfg.Network.IP)
	out, err := runVirsh(uri, "net-update", "default", "add", "ip-dhcp-host",
		dhcpHostXML(cfg.NetworkMAC, cfg.Network.IP), "--live", "--config")
	if err != nil {
		return fmt.Errorf("reserve IP %s for VM %q: %w: %s", cfg.Network.IP, cfg.Name, err, strings.TrimSpace(out))
	}
	return nil
}

func removeDHCPReservation(uri, network, mac, ip string) error {
	if network == "" || mac == "" || ip == "" {
		return nil
	}
	host := dhcpHostXML(mac, ip)
	out, err := runVirsh(uri, "net-update", network, "delete", "ip-dhcp-host", host, "--live", "--config")
	if err == nil {
		return nil
	}
	out2, err2 := runVirsh(uri, "net-update", network, "delete", "ip-dhcp-host", host, "--config")
	if err2 != nil {
		return fmt.Errorf("remove DHCP reservation: %w: %s", err2, strings.TrimSpace(out+out2))
	}
	return nil
}
