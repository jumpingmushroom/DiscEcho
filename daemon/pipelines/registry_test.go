package pipelines_test

import (
	"context"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

type fakeHandler struct {
	dt state.DiscType
}

func (f *fakeHandler) DiscType() state.DiscType { return f.dt }
func (f *fakeHandler) Identify(_ context.Context, _ *state.Drive) (*state.Disc, []state.Candidate, error) {
	return nil, nil, nil
}
func (f *fakeHandler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	return nil
}
func (f *fakeHandler) Run(_ context.Context, _ *state.Drive, _ *state.Disc, _ *state.Profile, _ pipelines.EventSink) error {
	return nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := pipelines.NewRegistry()
	r.Register(&fakeHandler{dt: state.DiscTypeAudioCD})

	got, ok := r.Get(state.DiscTypeAudioCD)
	if !ok {
		t.Fatal("not found")
	}
	if got.DiscType() != state.DiscTypeAudioCD {
		t.Errorf("got %s", got.DiscType())
	}

	if _, ok := r.Get(state.DiscTypeDVD); ok {
		t.Errorf("DVD should not be found")
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	r := pipelines.NewRegistry()
	r.Register(&fakeHandler{dt: state.DiscTypeAudioCD})

	defer func() {
		if recover() == nil {
			t.Errorf("want panic on duplicate register")
		}
	}()
	r.Register(&fakeHandler{dt: state.DiscTypeAudioCD})
}
