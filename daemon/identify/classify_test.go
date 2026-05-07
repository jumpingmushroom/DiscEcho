package identify_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestClassify_AudioCDFromCDInfo(t *testing.T) {
	out, err := os.ReadFile("testdata/cdinfo-cdda.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := identify.ClassifyFromCDInfo(string(out))
	if err != nil {
		t.Fatal(err)
	}
	if got != state.DiscTypeAudioCD {
		t.Errorf("want AUDIO_CD, got %s", got)
	}
}

func TestClassify_DataFromCDInfo(t *testing.T) {
	out, err := os.ReadFile("testdata/cdinfo-data.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := identify.ClassifyFromCDInfo(string(out))
	if err != nil {
		t.Fatal(err)
	}
	// cd-info reports DATA for any non-CD-DA disc; the higher-level
	// classifier consults the filesystem to refine to DVD/BDMV/UHD.
	if got != state.DiscTypeData {
		t.Errorf("want DATA, got %s", got)
	}
}

func TestClassify_EmptyCDInfo(t *testing.T) {
	if _, err := identify.ClassifyFromCDInfo(""); err == nil {
		t.Errorf("want error on empty input")
	}
}

// fakeFSProber returns a fixed file list.
type fakeFSProber struct{ files []string }

func (f *fakeFSProber) List(_ context.Context, _ string) ([]string, error) {
	return f.files, nil
}

// fakeBDProber returns a fixed BDInfo.
type fakeBDProber struct {
	info *identify.BDInfo
	err  error
}

func (f *fakeBDProber) Probe(_ context.Context, _ string) (*identify.BDInfo, error) {
	return f.info, f.err
}

func TestRefineDiscType_DVD(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/AUDIO_TS", "/VIDEO_TS", "/VIDEO_TS/VIDEO_TS.IFO"}},
		&fakeBDProber{},
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	if got != state.DiscTypeDVD {
		t.Errorf("want DVD, got %s", got)
	}
}

func TestRefineDiscType_BDMV(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/BDMV", "/BDMV/index.bdmv", "/CERTIFICATE"}},
		&fakeBDProber{info: &identify.BDInfo{AACSEncrypted: true, HasAACS2: false}},
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	if got != state.DiscTypeBDMV {
		t.Errorf("want BDMV, got %s", got)
	}
}

func TestRefineDiscType_UHD(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/BDMV", "/BDMV/index.bdmv", "/AACS"}},
		&fakeBDProber{info: &identify.BDInfo{AACSEncrypted: true, HasAACS2: true}},
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	if got != state.DiscTypeUHD {
		t.Errorf("want UHD, got %s", got)
	}
}

func TestRefineDiscType_BDMVWhenBDInfoFails(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/BDMV/index.bdmv"}},
		&fakeBDProber{err: errors.New("bd_info crashed")},
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	// Conservative: when bd_info is unavailable, default to BDMV. The
	// UHD handler's key-file precheck is the authoritative gate.
	if got != state.DiscTypeBDMV {
		t.Errorf("want BDMV (bd_info failure default), got %s", got)
	}
}

func TestRefineDiscType_DataWhenNoVideoMarkers(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/README.TXT", "/ARCHIVE/PHOTO_001.JPG"}},
		&fakeBDProber{},
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	if got != state.DiscTypeData {
		t.Errorf("want DATA, got %s", got)
	}
}

func TestRefineDiscType_PreservesAudioCD(t *testing.T) {
	// Audio CDs short-circuit before the fs probe; RefineDiscType
	// passes them through unchanged.
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeAudioCD,
		nil,
		nil,
		nil, // SystemCNFProber not needed for this case
		"/dev/sr0",
	)
	if got != state.DiscTypeAudioCD {
		t.Errorf("want AUDIO_CD, got %s", got)
	}
}

// fakeSystemCNFProber returns a fixed SystemCNF (or nil with err).
type fakeSystemCNFProber struct {
	info *identify.SystemCNF
	err  error
}

func (f *fakeSystemCNFProber) Probe(_ context.Context, _ string) (*identify.SystemCNF, error) {
	return f.info, f.err
}

func TestRefineDiscType_PSX(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/SYSTEM.CNF"}},
		&fakeBDProber{},
		&fakeSystemCNFProber{info: &identify.SystemCNF{BootCode: "SCUS_004.34", IsPS2: false}},
		"/dev/sr0",
	)
	if got != state.DiscTypePSX {
		t.Errorf("want PSX, got %s", got)
	}
}

func TestRefineDiscType_PS2(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/SYSTEM.CNF"}},
		&fakeBDProber{},
		&fakeSystemCNFProber{info: &identify.SystemCNF{BootCode: "SCES_500.51", IsPS2: true}},
		"/dev/sr0",
	)
	if got != state.DiscTypePS2 {
		t.Errorf("want PS2, got %s", got)
	}
}

func TestRefineDiscType_DataWhenSystemCNFUnreadable(t *testing.T) {
	got := identify.RefineDiscType(
		context.Background(),
		state.DiscTypeData,
		&fakeFSProber{files: []string{"/SYSTEM.CNF"}},
		&fakeBDProber{},
		&fakeSystemCNFProber{err: errors.New("isoinfo crashed")},
		"/dev/sr0",
	)
	if got != state.DiscTypeData {
		t.Errorf("want DATA (system.cnf unreadable fallback), got %s", got)
	}
}

func TestClassifier_Interface(t *testing.T) {
	c := identify.NewClassifier(identify.ClassifierConfig{})
	if c == nil {
		t.Fatal("nil classifier")
	}
	_, _ = c.Classify(context.Background(), "/dev/null")

	_, err := identify.NewClassifier(identify.ClassifierConfig{
		CDInfoBin: "/usr/bin/false",
	}).Classify(context.Background(), "/dev/null")
	if err == nil {
		t.Errorf("want error from /usr/bin/false")
	}
	if errors.Is(err, identify.ErrUnknownDiscType) {
		t.Errorf("classifier should not treat exec failure as unknown type")
	}
}
