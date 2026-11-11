package wisemedai

import (
	"bytes"
	"encoding/base64"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"image/png"
)

func pairingQR(token string) string {
	code, e := qr.Encode("wisemed-ai://pair?token="+token, qr.M, qr.Auto)
	if e != nil {
		return ""
	}
	img, e := barcode.Scale(code, 256, 256)
	if e != nil {
		return ""
	}
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
