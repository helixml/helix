package drm

import (
	"reflect"
	"testing"
)

// virtioPlanes builds a virtio-gpu style plane list: 2 planes per CRTC
// (primary, cursor), IDs stepping by 7, each usable only with its own CRTC.
func virtioPlanes(firstPrimary uint32, crtcs int) []planeInfo {
	var planes []planeInfo
	for i := 0; i < crtcs; i++ {
		base := firstPrimary + uint32(i)*7
		planes = append(planes,
			planeInfo{ID: base, PossibleCrtcs: 1 << i},
			planeInfo{ID: base + 1, PossibleCrtcs: 1 << i})
	}
	return planes
}

func TestGroupPlanesByCRTC(t *testing.T) {
	tests := []struct {
		name   string
		planes []planeInfo
		want   map[int][]uint32
	}{
		{
			name:   "kernel 6.17",
			planes: virtioPlanes(33, 3),
			want:   map[int][]uint32{0: {33, 34}, 1: {40, 41}, 2: {47, 48}},
		},
		{
			name:   "kernel 7.0 (all IDs +1)",
			planes: virtioPlanes(34, 3),
			want:   map[int][]uint32{0: {34, 35}, 1: {41, 42}, 2: {48, 49}},
		},
		{
			name: "unsorted input",
			planes: []planeInfo{
				{ID: 42, PossibleCrtcs: 1 << 1},
				{ID: 35, PossibleCrtcs: 1 << 0},
				{ID: 41, PossibleCrtcs: 1 << 1},
				{ID: 34, PossibleCrtcs: 1 << 0},
			},
			want: map[int][]uint32{0: {34, 35}, 1: {41, 42}},
		},
		{
			name:   "plane usable with multiple CRTCs",
			planes: []planeInfo{{ID: 10, PossibleCrtcs: 0b11}},
			want:   map[int][]uint32{0: {10}, 1: {10}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupPlanesByCRTC(tt.planes); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlaneIDsForScanout(t *testing.T) {
	m := &Manager{planeIDs: groupPlanesByCRTC(virtioPlanes(34, 3))}
	primary, cursor := m.planeIDsForScanout(1)
	if primary != 41 || cursor != 42 {
		t.Errorf("got primary=%d cursor=%d, want 41/42", primary, cursor)
	}
}

func TestValidatePlanes(t *testing.T) {
	crtcIDs := []uint32{38, 45, 52}
	scanouts := []uint32{1, 2}

	if err := validatePlanes(groupPlanesByCRTC(virtioPlanes(34, 3)), crtcIDs, scanouts); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	missingCursor := groupPlanesByCRTC(virtioPlanes(34, 3))
	missingCursor[2] = missingCursor[2][:1]
	if err := validatePlanes(missingCursor, crtcIDs, scanouts); err == nil {
		t.Error("expected error when scanout 2 has only one plane")
	}

	if err := validatePlanes(map[int][]uint32{}, crtcIDs, scanouts); err == nil {
		t.Error("expected error when no planes were enumerated")
	}
}
