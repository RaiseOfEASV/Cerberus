// Command vuln-verify-report checks an exported VulnMan report offline:
//
//	vuln-verify-report report.json public-key.b64
//
// report.json is the file downloaded from /api/reports; public-key.b64 is
// the "publicKey" value from /api/reports/public-key?version=<keyVersion>.
// Prints OK (exit 0) or TAMPERED (exit 1); exit 2 on usage or read errors.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

func run(args []string, out io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(out, "usage: vuln-verify-report <report.json> <public-key.b64>")
		return 2
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(out, "read report:", err)
		return 2
	}
	var rep struct {
		ReportJSON string `json:"reportJson"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &rep); err != nil || rep.Signature == "" {
		fmt.Fprintln(out, "not a VulnMan report export")
		return 2
	}
	keyText, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(out, "read public key:", err)
		return 2
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyText)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		fmt.Fprintln(out, "public key must be base64 Ed25519")
		return 2
	}
	if err := crypt.VerifyReport(ed25519.PublicKey(pub), []byte(rep.ReportJSON), rep.Signature); err != nil {
		fmt.Fprintln(out, "TAMPERED")
		return 1
	}
	fmt.Fprintln(out, "OK")
	return 0
}
