package model

import "testing"

func TestIsChannelHealthUnavailableForGroupStateIgnoresOtherGroups(t *testing.T) {
	globalState := &ChannelHealthState{
		Status:       ChannelHealthStatusUnhealthy,
		FailureCount: 10,
		LastGroup:    "default",
	}

	if IsChannelHealthUnavailableForGroupState(globalState, nil, "lzz_plus", 3) {
		t.Fatal("default/global probe failure must not make lzz_plus unavailable")
	}

	groupState := &ChannelHealthGroupState{
		GroupName:     "lzz_plus",
		FailureCount:  1,
		LastFailureAt: 100,
	}
	if IsChannelHealthUnavailableForGroupState(globalState, groupState, "lzz_plus", 3) {
		t.Fatal("group should remain routable before its threshold is reached")
	}

	groupState.FailureCount = 3
	if !IsChannelHealthUnavailableForGroupState(globalState, groupState, "lzz_plus", 3) {
		t.Fatal("group should be unavailable after reaching its threshold")
	}

	if IsChannelHealthUnavailableForGroupState(globalState, groupState, "lzz_plus", 0) {
		t.Fatal("non-positive threshold should keep the group routable")
	}
}
