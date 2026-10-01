package phpx

import "testing"

func TestHashes(t *testing.T) {
	for _, c := range [][3]int{{0, 0, 0}, {-1, -64, -1}, {30000000, 319, -30000000}, {-5, 70, 12}} {
		if got := WorldGetBlockXYZ(WorldBlockHash(c[0], c[1], c[2])); got != c {
			t.Errorf("block %v -> %v", c, got)
		}
		if got := WorldGetXZ(WorldChunkHash(c[0], c[2])); got != [2]int{c[0], c[2]} {
			t.Errorf("chunk %v -> %v", c, got)
		}
	}
}
