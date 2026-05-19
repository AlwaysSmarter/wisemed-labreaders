//go:build windows

package barcodeprinter

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

func sendToPrinter(printerName string, data []byte) error {
	if strings.TrimSpace(printerName) == "" {
		printerName = "default"
	}
	payload := base64.StdEncoding.EncodeToString(data)
	script := `
$pname=$env:WMR_PRINTER
$b64=$env:WMR_ZPL_B64
$bytes=[Convert]::FromBase64String($b64)

function Normalize-PrinterToken([string]$value) {
  if ([string]::IsNullOrWhiteSpace($value)) { return "" }
  $text = $value.Trim()
  if ($text.StartsWith("\\")) {
    $parts = $text -split "\\"
    if ($parts.Length -gt 0) {
      $leaf = $parts[$parts.Length - 1].Trim()
      if (-not [string]::IsNullOrWhiteSpace($leaf)) {
        $text = $leaf
      }
    }
  }
  if ($text -match "^(.*)\s+on\s+.+$") {
    $base = $matches[1].Trim()
    if (-not [string]::IsNullOrWhiteSpace($base)) {
      return $base.ToLowerInvariant()
    }
  }
  return $text.ToLowerInvariant()
}

function First-NonEmpty([string]$a, [string]$b) {
  if (-not [string]::IsNullOrWhiteSpace($a)) { return $a.Trim() }
  if (-not [string]::IsNullOrWhiteSpace($b)) { return $b.Trim() }
  return ""
}

function Get-PrinterHost([string]$value) {
  if ([string]::IsNullOrWhiteSpace($value)) { return "" }
  $text = $value.Trim()
  if ($text.StartsWith("\\")) {
    $parts = $text -split "\\"
    if ($parts.Length -ge 3) {
      return $parts[2].Trim()
    }
  }
  if ($text -match "^.*\s+on\s+(.+)$") {
    return $matches[1].Trim()
  }
  return ""
}

function Get-PrinterCatalog() {
  $items = New-Object System.Collections.Generic.List[object]
  $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)

  try {
    Get-Printer | ForEach-Object {
      $name = [string]$_.Name
      if ([string]::IsNullOrWhiteSpace($name)) { return }
      if ($seen.Add($name)) {
        $items.Add([pscustomobject]@{
          Name = $name.Trim()
          ShareName = ([string]$_.ShareName).Trim()
          HostName = Get-PrinterHost $name
          NormalizedName = Normalize-PrinterToken $name
          NormalizedShare = Normalize-PrinterToken ([string]$_.ShareName)
        })
      }
    }
  } catch {}

  try {
    Get-CimInstance Win32_Printer | ForEach-Object {
      $name = [string]$_.Name
      if ([string]::IsNullOrWhiteSpace($name)) { return }
      if ($seen.Add($name)) {
        $items.Add([pscustomobject]@{
          Name = $name.Trim()
          ShareName = ([string]$_.ShareName).Trim()
          HostName = First-NonEmpty (Get-PrinterHost $name) ([string]$_.SystemName)
          NormalizedName = Normalize-PrinterToken $name
          NormalizedShare = Normalize-PrinterToken ([string]$_.ShareName)
        })
      }
    }
  } catch {}

  return $items
}

function Resolve-PrinterName([string]$requested, $catalog) {
  if ([string]::IsNullOrWhiteSpace($requested)) { return $null }
  $requested = $requested.Trim()
  $normalized = Normalize-PrinterToken $requested

  $exact = $catalog | Where-Object {
    $_.Name -ieq $requested -or
    (-not [string]::IsNullOrWhiteSpace($_.ShareName) -and $_.ShareName -ieq $requested)
  } | Select-Object -First 1
  if ($exact) { return $exact }

  $normalizedMatches = $catalog | Where-Object {
    $_.NormalizedName -eq $normalized -or
    (-not [string]::IsNullOrWhiteSpace($_.NormalizedShare) -and $_.NormalizedShare -eq $normalized)
  }
  if (($normalizedMatches | Measure-Object).Count -eq 1) {
    return ($normalizedMatches | Select-Object -First 1)
  }

  $containsMatches = $catalog | Where-Object {
    $_.NormalizedName -like "*$normalized*" -or
    (-not [string]::IsNullOrWhiteSpace($_.NormalizedShare) -and $_.NormalizedShare -like "*$normalized*")
  }
  if (($containsMatches | Measure-Object).Count -eq 1) {
    return ($containsMatches | Select-Object -First 1)
  }

  return $null
}

function Get-OpenCandidates([string]$requested, $entry) {
  $list = New-Object System.Collections.Generic.List[string]
  $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
  $baseName = ""
  if ($entry -ne $null) {
    $baseName = $entry.Name
  }
  foreach ($value in @(
    $requested,
    $entry.Name,
    $entry.ShareName,
    ("\\" + $entry.HostName + "\" + $entry.ShareName),
    ("\\" + $entry.HostName + "\" + $baseName)
  )) {
    if ([string]::IsNullOrWhiteSpace($value)) { continue }
    $text = $value.Trim()
    if ($seen.Add($text)) {
      $list.Add($text)
    }
  }
  return $list
}

function Ensure-PrinterConnection([string]$candidate) {
  if ([string]::IsNullOrWhiteSpace($candidate)) { return $false }
  $value = $candidate.Trim()
  if (-not $value.StartsWith("\\")) { return $false }
  try {
    Add-Printer -ConnectionName $value -ErrorAction Stop | Out-Null
    Start-Sleep -Milliseconds 300
    return $true
  } catch {
    return $false
  }
}

$catalog = Get-PrinterCatalog
$resolved = $null
if ($pname -eq "default" -or [string]::IsNullOrWhiteSpace($pname)) {
  $default = Get-CimInstance Win32_Printer | Where-Object { $_.Default } | Select-Object -First 1 -ExpandProperty Name
  if ([string]::IsNullOrWhiteSpace($default)) {
    throw "default printer not found"
  }
  $pname = $default
} else {
  $resolved = Resolve-PrinterName $pname $catalog
  if ($null -eq $resolved) {
    $available = ($catalog | ForEach-Object {
      if ([string]::IsNullOrWhiteSpace($_.ShareName)) { $_.Name } else { $_.Name + " [share: " + $_.ShareName + "]" }
    }) -join "; "
    throw "printer not found for input '$pname'. visible printers: $available"
  }
}

$signature = @"
using System;
using System.Runtime.InteropServices;

public static class RawPrinter {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public class DOCINFO {
    [MarshalAs(UnmanagedType.LPWStr)]
    public string pDocName;
    [MarshalAs(UnmanagedType.LPWStr)]
    public string pOutputFile;
    [MarshalAs(UnmanagedType.LPWStr)]
    public string pDataType;
  }

  [DllImport("winspool.drv", SetLastError = true, CharSet = CharSet.Unicode)]
  public static extern bool OpenPrinter(string pPrinterName, out IntPtr phPrinter, IntPtr pDefault);

  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool ClosePrinter(IntPtr hPrinter);

  [DllImport("winspool.drv", SetLastError = true, CharSet = CharSet.Unicode)]
  public static extern bool StartDocPrinter(IntPtr hPrinter, Int32 level, DOCINFO di);

  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool EndDocPrinter(IntPtr hPrinter);

  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool StartPagePrinter(IntPtr hPrinter);

  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool EndPagePrinter(IntPtr hPrinter);

  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool WritePrinter(IntPtr hPrinter, byte[] pBytes, Int32 dwCount, out Int32 dwWritten);
}
"@

Add-Type -TypeDefinition $signature

$handle = [IntPtr]::Zero
$openCandidates = @($pname)
if ($null -ne $resolved) {
  $openCandidates = Get-OpenCandidates $pname $resolved
}
foreach ($candidate in $openCandidates) {
  if ([RawPrinter]::OpenPrinter($candidate, [ref]$handle, [IntPtr]::Zero)) {
    $pname = $candidate
    break
  }
  if (Ensure-PrinterConnection $candidate) {
    if ([RawPrinter]::OpenPrinter($candidate, [ref]$handle, [IntPtr]::Zero)) {
      $pname = $candidate
      break
    }
  }
}
if ($handle -eq [IntPtr]::Zero) {
  $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
  throw "OpenPrinter failed: $([Runtime.InteropServices.Marshal]::GetLastWin32Error()) user=$identity attempted names: $(($openCandidates -join '; '))"
}
try {
  $doc = New-Object RawPrinter+DOCINFO
  $doc.pDocName = "WiseMED Barcode ZPL"
  $doc.pDataType = "RAW"
  if (-not [RawPrinter]::StartDocPrinter($handle, 1, $doc)) {
    throw "StartDocPrinter failed: $([Runtime.InteropServices.Marshal]::GetLastWin32Error())"
  }
  try {
    if (-not [RawPrinter]::StartPagePrinter($handle)) {
      throw "StartPagePrinter failed: $([Runtime.InteropServices.Marshal]::GetLastWin32Error())"
    }
    try {
      $written = 0
      if (-not [RawPrinter]::WritePrinter($handle, $bytes, $bytes.Length, [ref]$written)) {
        throw "WritePrinter failed: $([Runtime.InteropServices.Marshal]::GetLastWin32Error())"
      }
      if ($written -ne $bytes.Length) {
        throw "WritePrinter incomplete: wrote $written of $($bytes.Length) bytes"
      }
    } finally {
      [void][RawPrinter]::EndPagePrinter($handle)
    }
  } finally {
    [void][RawPrinter]::EndDocPrinter($handle)
  }
} finally {
  [void][RawPrinter]::ClosePrinter($handle)
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(cmd.Env, "WMR_PRINTER="+printerName, "WMR_ZPL_B64="+payload)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s: %w", msg, err)
	}
	return nil
}

func listPrinters() []string {
	script := `
$names = New-Object System.Collections.Generic.List[string]

try {
  Get-Printer | ForEach-Object {
    if (-not [string]::IsNullOrWhiteSpace($_.Name)) {
      $names.Add([string]$_.Name)
    }
  }
} catch {}

try {
  Get-CimInstance Win32_Printer | ForEach-Object {
    foreach ($value in @($_.Name, $_.ShareName)) {
      if ($value -is [string] -and -not [string]::IsNullOrWhiteSpace($value)) {
        $names.Add($value)
      }
    }
  }
} catch {}

$names |
  Where-Object { -not [string]::IsNullOrWhiteSpace($_) } |
  ForEach-Object { $_.Trim() } |
  Sort-Object -Unique
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}
	lines := strings.Split(string(out), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names
}
