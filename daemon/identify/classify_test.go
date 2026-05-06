package identify_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestClassify_AudioCD(t *testing.T) {
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

func TestClassify_Data(t *testing.T) {
	out, err := os.ReadFile("testdata/cdinfo-data.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := identify.ClassifyFromCDInfo(string(out))
	if err != nil {
		t.Fatal(err)
	}
	if got != state.DiscTypeData {
		t.Errorf("want DATA, got %s", got)
	}
}

func TestClassify_Empty(t *testing.T) {
	_, err := identify.ClassifyFromCDInfo("")
	if err == nil {
		t.Errorf("want error on empty input")
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
