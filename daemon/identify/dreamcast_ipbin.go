package identify

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// DCIPBin is the parsed Dreamcast IP.BIN header. ProductNumber is the
// BootCodeIndex lookup key (after uppercasing/trimming). See
// https://mc.pp.se/dc/ip.bin.html for the field layout.
type DCIPBin struct {
	HardwareID    string
	MakerID       string
	DeviceInfo    string
	AreaSymbols   string
	Peripherals   string
	ProductNumber string
	Version       string
	ReleaseDate   string
	BootFilename  string
	SoftwareMaker string
	SoftwareName  string
}

const (
	// dcIPBinLBA is the GD-ROM sector where the IP.BIN header lives. Sega
	// reserves the first 45000 sectors of a GD-ROM for the CD-area / TOC
	// games; the HD area starts here. Standard sector size 2048 bytes.
	dcIPBinLBA       = 45000
	dcSectorSize     = 2048
	dcIPBinByteOffs  = dcIPBinLBA * dcSectorSize
	dcIPBinReadBytes = 256
	dcHardwareIDExp  = "SEGA SEGAKATANA "
)

// ParseDCIPBin decodes a 256-byte IP.BIN header into its fields. Returns
// nil if the input is too short or the hardware-ID doesn't match the
// Dreamcast magic — defensive against running this on a non-DC disc by
// mistake.
func ParseDCIPBin(raw []byte) *DCIPBin {
	if len(raw) < dcIPBinReadBytes {
		return nil
	}
	if string(raw[0:16]) != dcHardwareIDExp {
		return nil
	}
	trim := func(b []byte) string { return strings.TrimRight(string(b), " \x00") }
	return &DCIPBin{
		HardwareID:    trim(raw[0x000:0x010]),
		MakerID:       trim(raw[0x010:0x020]),
		DeviceInfo:    trim(raw[0x020:0x030]),
		AreaSymbols:   trim(raw[0x030:0x038]),
		Peripherals:   trim(raw[0x038:0x040]),
		ProductNumber: trim(raw[0x040:0x04A]),
		Version:       trim(raw[0x04A:0x050]),
		ReleaseDate:   trim(raw[0x050:0x060]),
		BootFilename:  trim(raw[0x060:0x070]),
		SoftwareMaker: trim(raw[0x070:0x080]),
		SoftwareName:  trim(raw[0x080:0x100]),
	}
}

// DCIPBinReader reads the 256-byte IP.BIN header from a Dreamcast disc.
type DCIPBinReader interface {
	Read(ctx context.Context, devPath string) (*DCIPBin, error)
}

// NewDCIPBinReader returns a reader that opens the block device and
// reads 256 bytes from LBA 45000.
func NewDCIPBinReader() DCIPBinReader {
	return &deviceDCIPBinReader{}
}

type deviceDCIPBinReader struct{}

func (r *deviceDCIPBinReader) Read(_ context.Context, devPath string) (*DCIPBin, error) {
	f, err := os.OpenFile(devPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("dc ip.bin: open %s: %w", devPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(int64(dcIPBinByteOffs), io.SeekStart); err != nil {
		return nil, fmt.Errorf("dc ip.bin: seek to LBA %d: %w", dcIPBinLBA, err)
	}
	buf := make([]byte, dcIPBinReadBytes)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("dc ip.bin: read 256 bytes: %w", err)
	}
	info := ParseDCIPBin(buf)
	if info == nil {
		return nil, fmt.Errorf("dc ip.bin: header at LBA %d is not a Dreamcast IP.BIN", dcIPBinLBA)
	}
	return info, nil
}
