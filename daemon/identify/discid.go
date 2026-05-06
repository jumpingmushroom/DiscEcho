package identify

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
)

// DiscID computes the MusicBrainz disc ID for a CD's TOC.
//
// Algorithm (from https://musicbrainz.org/doc/Disc_ID_Calculation):
//  1. Build the ASCII string:
//     <first:%02X><last:%02X><offset[0]:%08X>...<offset[99]:%08X>
//     where offset[0] is the lead-out LBA and offset[1..99] are the
//     per-track LBAs. Slots beyond the last real track are 00000000.
//  2. SHA1 hash that string.
//  3. Base64-encode the digest using MB's URL-safe alphabet:
//     '+' → '.', '/' → '_', '=' → '-'.
//
// firstTrack and lastTrack are 1-indexed; leadoutLBA is the start of
// the lead-out track in sectors; lbas is in track order, one entry per
// real track.
func DiscID(firstTrack, lastTrack, leadoutLBA int, lbas []int) string {
	var sb strings.Builder
	sb.Grow(2 + 2 + 8*100)

	fmt.Fprintf(&sb, "%02X", firstTrack)
	fmt.Fprintf(&sb, "%02X", lastTrack)
	// offset[0] is lead-out; offset[i] for i>=1 is the LBA of track i.
	fmt.Fprintf(&sb, "%08X", leadoutLBA)
	for i := 1; i < 100; i++ {
		idx := i - 1
		if idx < len(lbas) {
			fmt.Fprintf(&sb, "%08X", lbas[idx])
		} else {
			sb.WriteString("00000000")
		}
	}

	sum := sha1.Sum([]byte(sb.String()))
	encoded := base64.StdEncoding.EncodeToString(sum[:])
	encoded = strings.NewReplacer("+", ".", "/", "_", "=", "-").Replace(encoded)
	return encoded
}
