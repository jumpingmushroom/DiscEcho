package drive

import "strings"

// Uevent is a parsed kernel uevent: action plus key/value properties.
type Uevent struct {
	Action     string
	DevPath    string
	Subsystem  string
	DevName    string
	DevType    string
	Properties map[string]string
}

// ParseUevent splits a uevent payload (newline- OR NUL-separated
// key=value lines) into a Uevent. The first line in real kernel
// payloads is the bare event header (e.g. "change@/devices/...");
// it is tolerated and ignored.
func ParseUevent(payload string) (Uevent, bool) {
	if payload == "" {
		return Uevent{}, false
	}
	// Normalise: kernel uses \x00 separators, tests use \n.
	payload = strings.ReplaceAll(payload, "\x00", "\n")
	lines := strings.Split(payload, "\n")
	ev := Uevent{Properties: make(map[string]string, 8)}
	for _, line := range lines {
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		ev.Properties[k] = v
		switch k {
		case "ACTION":
			ev.Action = v
		case "DEVPATH":
			ev.DevPath = v
		case "SUBSYSTEM":
			ev.Subsystem = v
		case "DEVNAME":
			// udevd-style events include the "/dev/" prefix; kernel-style
			// events don't. Normalize so callers can always build the
			// node path as "/dev/" + DevName.
			ev.DevName = strings.TrimPrefix(v, "/dev/")
		case "DEVTYPE":
			ev.DevType = v
		}
	}
	return ev, true
}

// IsOpticalMediaChange reports whether the event represents a
// disc-insert/remove on an optical drive (ID_CDROM=1, SUBSYSTEM=block,
// DISK_MEDIA_CHANGE=1).
func (e Uevent) IsOpticalMediaChange() bool {
	if e.Subsystem != "block" {
		return false
	}
	if e.Properties["ID_CDROM"] != "1" {
		return false
	}
	if e.Properties["DISK_MEDIA_CHANGE"] != "1" {
		return false
	}
	return true
}
