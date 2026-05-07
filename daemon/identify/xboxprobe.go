package identify

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotXbox is returned when default.xbe is missing or has no XBEH magic.
var ErrNotXbox = errors.New("identify: not an Xbox disc")

// XboxInfo is the subset of XBE certificate data the daemon consults.
type XboxInfo struct {
	TitleID uint32 // little-endian uint32 from XBE certificate
	Region  string // "USA", "Japan", "ROW", "World", or "" if no flags set
}

// ProbeXBE parses an XBE binary blob. The xbox pipeline uses this with
// `isoinfo -x /default.xbe` to identify a disc without mounting it.
//
// XBE layout we care about:
//   - magic "XBEH" at offset 0
//   - base address (uint32 LE) at 0x104
//   - certificate offset (uint32 LE, in XBE virtual address space) at 0x118
//   - inside the certificate: title ID at +0x8, regions at +0xC8
func ProbeXBE(data []byte) (*XboxInfo, error) {
	if len(data) < 0x180 || !bytes.HasPrefix(data, []byte("XBEH")) {
		return nil, ErrNotXbox
	}
	baseAddr := binary.LittleEndian.Uint32(data[0x104:])
	certVA := binary.LittleEndian.Uint32(data[0x118:])
	if certVA < baseAddr {
		return nil, fmt.Errorf("xbe: certificate VA %#x below base %#x", certVA, baseAddr)
	}
	certOff := uint64(certVA - baseAddr)
	if certOff+0xCC > uint64(len(data)) {
		return nil, fmt.Errorf("xbe: certificate truncated (off=%#x, file=%d bytes)", certOff, len(data))
	}
	titleID := binary.LittleEndian.Uint32(data[certOff+0x8:])
	regions := binary.LittleEndian.Uint32(data[certOff+0xC8:])
	return &XboxInfo{
		TitleID: titleID,
		Region:  xboxRegionString(regions),
	}, nil
}

// ProbeXbox reads <mountPoint>/default.xbe and parses the XBE header
// + certificate for title ID and allowed-region bitfield.
func ProbeXbox(mountPoint string) (*XboxInfo, error) {
	path := filepath.Join(mountPoint, "default.xbe")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotXbox
		}
		return nil, fmt.Errorf("read default.xbe: %w", err)
	}
	return ProbeXBE(data)
}

func xboxRegionString(flags uint32) string {
	const (
		usa = 0x01
		jpn = 0x02
		row = 0x04
	)
	mask := flags & (usa | jpn | row)
	switch mask {
	case usa:
		return "USA"
	case jpn:
		return "Japan"
	case row:
		return "ROW"
	case usa | jpn | row:
		return "World"
	case 0:
		return ""
	default:
		return fmt.Sprintf("region:%x", mask)
	}
}
