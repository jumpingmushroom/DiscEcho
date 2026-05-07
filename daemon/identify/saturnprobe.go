package identify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrNotSaturn is returned by Saturn probes when the IP.BIN magic
// signature is not present at the start of sector 0.
var ErrNotSaturn = errors.New("identify: not a Saturn disc")

// SaturnInfo is the subset of the Saturn IP.BIN we consult for
// identification + Redump dat lookup.
type SaturnInfo struct {
	ProductNumber string // e.g. "MK-81088"
	Version       string // e.g. "V1.000"
	Region        string // compatible-area string, e.g. "JTUBKAEL"
}

// ProbeSaturn opens devPath, reads the first 256 bytes of sector 0,
// and parses Saturn IP.BIN. Returns ErrNotSaturn when the magic does
// not match.
func ProbeSaturn(devPath string) (*SaturnInfo, error) {
	f, err := os.Open(devPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", devPath, err)
	}
	defer func() { _ = f.Close() }()
	return ProbeSaturnReader(f)
}

// DevSaturnProber implements SaturnProber by opening the device path
// and delegating to ProbeSaturnReader.
type DevSaturnProber struct{}

// NewDevSaturnProber returns a DevSaturnProber ready for use.
func NewDevSaturnProber() *DevSaturnProber { return &DevSaturnProber{} }

// Probe reads Saturn IP.BIN from devPath.
func (*DevSaturnProber) Probe(_ context.Context, devPath string) (*SaturnInfo, error) {
	return ProbeSaturn(devPath)
}

// ProbeSaturnReader is the testable core of ProbeSaturn — separated so
// tests don't need a real device.
func ProbeSaturnReader(r io.Reader) (*SaturnInfo, error) {
	buf := make([]byte, 256)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read ip.bin: %w", err)
	}
	if !bytes.HasPrefix(buf, []byte("SEGA SEGASATURN")) {
		return nil, ErrNotSaturn
	}
	return &SaturnInfo{
		ProductNumber: strings.TrimSpace(string(buf[0x20 : 0x20+10])),
		Version:       strings.TrimSpace(string(buf[0x2A : 0x2A+6])),
		Region:        strings.TrimSpace(string(buf[0x40 : 0x40+10])),
	}, nil
}
