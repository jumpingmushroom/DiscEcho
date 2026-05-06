// Package identify reads optical-disc TOCs, looks up metadata from
// MusicBrainz, and classifies a disc into one of the canonical types
// (AUDIO_CD, DATA, etc.) so the orchestrator can pick the right
// pipeline handler.
//
// Pure-Go where possible; subprocess shell-outs (cdparanoia, cd-info)
// for the parts that talk to libcdio. The Reader interfaces let Phase
// D's mock-tool tests substitute fakes.
package identify

import "context"

// Track is one row of an optical disc's table of contents.
type Track struct {
	Number    int  // 1-indexed
	StartLBA  int  // sector offset of the first frame of this track
	LengthLBA int  // sector count
	IsAudio   bool // false for data tracks (mixed-mode CDs)
}

// TOC is the parsed table of contents. LeadoutLBA is one past the last
// playable sector (i.e. start of lead-out, used by the MB disc-ID hash).
type TOC struct {
	Tracks     []Track
	LeadoutLBA int
}

// FirstTrack returns the lowest track number in the TOC, or 0 if empty.
func (t TOC) FirstTrack() int {
	if len(t.Tracks) == 0 {
		return 0
	}
	return t.Tracks[0].Number
}

// LastTrack returns the highest track number in the TOC, or 0 if empty.
func (t TOC) LastTrack() int {
	if len(t.Tracks) == 0 {
		return 0
	}
	return t.Tracks[len(t.Tracks)-1].Number
}

// TOCReader reads the table of contents from an optical drive.
// Implementations: tocCDParanoia (production), fakes (tests).
type TOCReader interface {
	Read(ctx context.Context, devPath string) (*TOC, error)
}
