package wrapper

import (
	"reflect"
	"testing"
)

func TestMergeProperties_DeepMergesTheMapsInsideAndShallowReplacesThem_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The named blocks of a configuration are merged in the order given, deep or shallow as the caller asks",
		CurrentImpl:     "MergeProperties copies each block over the ones before with maps.Copy, or hands them to the strategic merge engine when deep",
		ExpectedOutcome: "A later block wins either way, a map inside a block is merged key by key only when deep, a block that is missing is skipped and one that isn't a map is an error",
		Rationale:       "The secrets merge deep and a root's merge() is shallow, a bucket name made from the wrong one can't be changed afterwards",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: two blocks that both set a map, a block nobody asked for, and one name that isn't there at all
	content := map[string]interface{}{
		"example_global": map[string]interface{}{"owner": "global", "nested": map[string]interface{}{"a": "global", "b": "global"}},
		"example_proj":   map[string]interface{}{"owner": "proj", "nested": map[string]interface{}{"a": "proj"}},
		"example_other":  map[string]interface{}{"owner": "other"},
		"example_list":   []interface{}{"not", "a", "map"},
	}

	// When: they are merged both ways, in both orders, and once with a block that isn't a map
	shallow, shallowErr := MergeProperties(content, false, "example_global", "example_missing", "example_proj")
	deep, deepErr := MergeProperties(content, true, "example_global", "example_missing", "example_proj")
	reversed, reversedErr := MergeProperties(content, false, "example_proj", "example_global")
	_, listErr := MergeProperties(content, false, "example_global", "example_list")

	// Then: the last block has the last word, and only the deep merge keeps what the first one had inside the map
	if shallowErr != nil || deepErr != nil || reversedErr != nil {
		t.Fatal(shallowErr, deepErr, reversedErr)
	}
	wantShallow := map[string]interface{}{"owner": "proj", "nested": map[string]interface{}{"a": "proj"}}
	if !reflect.DeepEqual(shallow, wantShallow) {
		t.Errorf("shallow = %v, want %v", shallow, wantShallow)
	}
	wantDeep := map[string]interface{}{"owner": "proj", "nested": map[string]interface{}{"a": "proj", "b": "global"}}
	if !reflect.DeepEqual(deep, wantDeep) {
		t.Errorf("deep = %v, want %v", deep, wantDeep)
	}
	if reversed["owner"] != "global" {
		t.Errorf("reversed = %v, want the block named last to win", reversed)
	}
	if listErr == nil {
		t.Error("a block that isn't a map was merged")
	}
}
