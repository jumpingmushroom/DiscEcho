package api

import (
	"fmt"
	"text/template"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// OptionType is a tag for option-blob value types.
type OptionType string

const (
	OptString OptionType = "string"
	OptInt    OptionType = "int"
	OptBool   OptionType = "bool"
)

// OptionSchema declares one valid key in a profile's options blob.
type OptionSchema struct {
	Type     OptionType
	Required bool
}

// EngineSchema declares the constraints for one engine string. The
// daemon validates incoming profiles against the engine-keyed map at
// CreateProfile/UpdateProfile time.
type EngineSchema struct {
	Formats   []string
	Options   map[string]OptionSchema
	StepCount int
}

// engineSchemas is the canonical map. Hand-edited; changes here also
// need a parallel update in webui/src/lib/profile_schema.ts. Server is
// authoritative — clients drift, but bad input gets rejected.
var engineSchemas = map[string]EngineSchema{
	"whipper": {
		Formats:   []string{"FLAC"},
		Options:   map[string]OptionSchema{},
		StepCount: 6,
	},
	"MakeMKV": {
		Formats: []string{"MKV"},
		Options: map[string]OptionSchema{
			"min_title_seconds": {Type: OptInt},
			"keep_all_tracks":   {Type: OptBool},
		},
		StepCount: 6,
	},
	"MakeMKV+HandBrake": {
		Formats: []string{"MKV"},
		Options: map[string]OptionSchema{
			"min_title_seconds": {Type: OptInt},
			"keep_all_tracks":   {Type: OptBool},
		},
		StepCount: 7,
	},
	"HandBrake": {
		Formats: []string{"MP4", "MKV"},
		Options: map[string]OptionSchema{
			"min_title_seconds": {Type: OptInt},
			"season":            {Type: OptInt},
		},
		StepCount: 7,
	},
	"redumper+chdman": {
		Formats:   []string{"CHD"},
		Options:   map[string]OptionSchema{},
		StepCount: 7,
	},
	"redumper": {
		Formats:   []string{"ISO"},
		Options:   map[string]OptionSchema{},
		StepCount: 5,
	},
	"dd": {
		Formats:   []string{"ISO"},
		Options:   map[string]OptionSchema{},
		StepCount: 5,
	},
}

// ValidationError is one field-level issue with a submitted profile.
type ValidationError struct {
	Field string `json:"field"`
	Msg   string `json:"msg"`
}

// ValidateProfile checks p against engineSchemas. Returns a slice of
// field-specific errors (empty when valid).
//
// Rules:
//   - Name + DiscType + Engine required.
//   - Engine must exist in engineSchemas.
//   - Format must be in schema.Formats.
//   - Each Options key must exist in schema.Options; the value's
//     runtime type must match (JSON-decoded ints become float64 — we
//     accept both for OptInt so the API works as users expect).
//   - OutputPathTemplate must parse via text/template.
func ValidateProfile(p *state.Profile) []ValidationError {
	var errs []ValidationError

	if p.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Msg: "required"})
	}
	if p.DiscType == "" {
		errs = append(errs, ValidationError{Field: "disc_type", Msg: "required"})
	}
	if p.Engine == "" {
		errs = append(errs, ValidationError{Field: "engine", Msg: "required"})
		return errs
	}
	schema, ok := engineSchemas[p.Engine]
	if !ok {
		errs = append(errs, ValidationError{
			Field: "engine",
			Msg:   fmt.Sprintf("unknown engine %q", p.Engine),
		})
		return errs
	}

	if !contains(schema.Formats, p.Format) {
		errs = append(errs, ValidationError{
			Field: "format",
			Msg:   fmt.Sprintf("engine %s requires format in %v, got %q", p.Engine, schema.Formats, p.Format),
		})
	}
	for k, v := range p.Options {
		opt, known := schema.Options[k]
		if !known {
			errs = append(errs, ValidationError{
				Field: "options." + k,
				Msg:   fmt.Sprintf("unknown option for engine %s", p.Engine),
			})
			continue
		}
		if !valueMatchesType(v, opt.Type) {
			errs = append(errs, ValidationError{
				Field: "options." + k,
				Msg:   fmt.Sprintf("expected %s", opt.Type),
			})
		}
	}
	if p.OutputPathTemplate != "" {
		if _, err := template.New("output").Parse(p.OutputPathTemplate); err != nil {
			errs = append(errs, ValidationError{
				Field: "output_path_template",
				Msg:   err.Error(),
			})
		}
	}
	return errs
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func valueMatchesType(v any, t OptionType) bool {
	switch t {
	case OptString:
		_, ok := v.(string)
		return ok
	case OptInt:
		// JSON decodes numbers as float64. Accept both.
		switch n := v.(type) {
		case int:
			return true
		case float64:
			return n == float64(int(n)) // whole number
		}
		return false
	case OptBool:
		_, ok := v.(bool)
		return ok
	}
	return false
}
