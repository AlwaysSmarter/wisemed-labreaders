//go:build windows

package siui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

const invoiceSignScript = `$ErrorActionPreference='Stop'
try {
 Add-Type -AssemblyName System.Security
 $p=[Console]::In.ReadToEnd()|ConvertFrom-Json
 if($p.store -ne 'CurrentUser' -and $p.store -ne 'LocalMachine'){throw 'Invalid store'}
 $store=New-Object System.Security.Cryptography.X509Certificates.X509Store('My',$p.store)
 $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
 try{
  $certs=$store.Certificates.Find([System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,$p.thumbprint,$true)
  if($certs.Count -ne 1 -or -not $certs[0].HasPrivateKey){throw 'Certificate unavailable'}
  $content=[System.Security.Cryptography.Pkcs.ContentInfo]::new([Convert]::FromBase64String($p.data))
  $cms=[System.Security.Cryptography.Pkcs.SignedCms]::new($content,$false)
  $signer=[System.Security.Cryptography.Pkcs.CmsSigner]::new($certs[0])
  $signer.DigestAlgorithm=[System.Security.Cryptography.Oid]::new('2.16.840.1.101.3.4.2.1')
  $cms.ComputeSignature($signer,$false)
  [Console]::Out.Write([Convert]::ToBase64String($cms.Encode()))
 }finally{$store.Close()}
}catch{[Console]::Error.Write('CMS token signing failed; check certificate and PIN dialog');exit 1}
`

func (n nativeBackend) SignInvoice(ctx context.Context, data []byte) ([]byte, error) {
	if result, active, err := n.signWithPIN(ctx, data); active {
		return result, err
	}
	input, _ := json.Marshal(map[string]string{"store": n.cfg.Store, "thumbprint": n.cfg.Thumbprint, "data": base64.StdEncoding.EncodeToString(data)})
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, executable, "-NoProfile", "-Command", invoiceSignScript)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.New("Windows CMS token signing failed; check certificate and PIN dialog")
	}
	return base64.StdEncoding.DecodeString(string(output))
}
