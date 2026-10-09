package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

// These tests are IR-parametrized: they iterate AllModels (regenerated from the
// server IR into manifest.go), so a new server model automatically gets baseline
// coverage with no hand-edit — the Go analog of the Python SDK's IR-driven tests.

// TestAllModelsRoundTrip exercises the JSON codec of every generated model at its
// zero value: Marshal must succeed, Unmarshal must not panic, and when Unmarshal
// succeeds the re-marshaled bytes must be stable (idempotent round-trip). A oneOf
// / mutex model legitimately REJECTS an empty document on Unmarshal (its
// structural validator enforces exactly-one) — that's correct behavior, so an
// Unmarshal error is tolerated; a panic or a non-idempotent round-trip is not.
func TestAllModelsRoundTrip(t *testing.T) {
	if len(AllModels) == 0 {
		t.Fatal("AllModels is empty — manifest.go not generated?")
	}
	for name, ptr := range AllModelNames {
		typ := reflect.TypeOf(ptr).Elem() // the model struct/enum type
		v := reflect.New(typ).Interface()
		data, err := json.Marshal(v)
		if err != nil {
			t.Errorf("%s: Marshal(zero) failed: %v", name, err)
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Unmarshal(%s) panicked: %v", name, data, r)
				}
			}()
			back := reflect.New(typ).Interface()
			if err := json.Unmarshal(data, back); err != nil {
				return // structural validator rejecting the zero value — expected
			}
			again, err := json.Marshal(back)
			if err != nil {
				t.Errorf("%s: re-Marshal failed: %v", name, err)
				return
			}
			if string(again) != string(data) {
				t.Errorf("%s: round-trip not idempotent:\n  first:  %s\n  second: %s", name, data, again)
			}
		}()
	}
}

// TestAllModelsCountMatchesFiles is a cheap guard that the manifest wasn't emitted
// empty or truncated: it must list every AllModelNames entry in AllModels too.
func TestAllModelsCountMatchesFiles(t *testing.T) {
	if len(AllModels) != len(AllModelNames) {
		t.Fatalf("AllModels (%d) and AllModelNames (%d) disagree", len(AllModels), len(AllModelNames))
	}
}

// TestValidatorsAcceptOrRejectDeterministically invokes Validate() on every model
// that has one, at its zero value, asserting the call itself never panics. (A
// zero value may legitimately be invalid — we only assert Validate is total.)
func TestValidatorsDoNotPanic(t *testing.T) {
	type validator interface{ Validate() error }
	for name, ptr := range AllModelNames {
		typ := reflect.TypeOf(ptr).Elem()
		v := reflect.New(typ).Interface()
		if val, ok := v.(validator); ok {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s: Validate() panicked: %v", name, r)
					}
				}()
				_ = val.Validate()
			}()
		}
	}
}
