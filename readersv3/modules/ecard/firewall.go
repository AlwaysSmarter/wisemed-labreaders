package ecard

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Only this application's named rule is managed. DNS is left untouched.
const firewallScript = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding
try {
$name='WiseMED-PIAS-eCard-CEAS-Offline'
$rule=Get-NetFirewallRule -PolicyStore PersistentStore -Name $name -ErrorAction SilentlyContinue
if ($env:WM_ECARD_OFFLINE -eq '1') {
 $ips=$env:WM_ECARD_IPS.Split(',')
 if ($rule) {
  $rule | Get-NetFirewallAddressFilter | Set-NetFirewallAddressFilter -RemoteAddress $ips | Out-Null
  $rule | Get-NetFirewallPortFilter | Set-NetFirewallPortFilter -Protocol TCP -RemotePort $env:WM_ECARD_PORT | Out-Null
  $rule | Set-NetFirewallRule -Enabled True -Direction Outbound -Action Block -Profile Any | Out-Null
 } else {
  New-NetFirewallRule -PolicyStore PersistentStore -Name $name -DisplayName 'WiseMED PIAS eCard - CEAS offline' -Direction Outbound -Action Block -Enabled True -Profile Any -Protocol TCP -RemoteAddress $ips -RemotePort $env:WM_ECARD_PORT | Out-Null
 }
} elseif ($rule) { $rule | Remove-NetFirewallRule }
} catch {
 [Console]::Error.WriteLine(($_ | Out-String))
 exit 1
}
`

// EncodedCommand avoids Windows command-line quoting of the script's newlines,
// quotes and pipelines. Windows PowerShell expects UTF-16LE Base64.
func encodedPowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func applyCEASFirewall(ctx context.Context, s Settings) error {
	if runtime.GOOS != "windows" {
		if s.CEASOffline {
			return errors.New("CEAS offline firewall is available on Windows only")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	enabled := "0"
	ips := []string{}
	if s.CEASOffline {
		enabled = "1"
		addresses, e := net.DefaultResolver.LookupIPAddr(ctx, s.UMHost)
		if e != nil || len(addresses) == 0 {
			return errors.New("cannot resolve UM for CEAS offline firewall")
		}
		for _, a := range addresses {
			ips = append(ips, a.IP.String())
		}
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPowerShell(firewallScript))
	cmd.Env = append(os.Environ(), "WM_ECARD_OFFLINE="+enabled, "WM_ECARD_IPS="+strings.Join(ips, ","), "WM_ECARD_PORT="+strconv.Itoa(s.UMPort))
	out, e := cmd.CombinedOutput()
	if e != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("cannot apply CEAS firewall: %w", ctx.Err())
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = "PowerShell returned no diagnostic output"
		}
		return fmt.Errorf("cannot apply CEAS firewall: %s (%w)", detail, e)
	}
	return nil
}
