package frontend

import (
	"reflect"
	"strings"
	"testing"
)

// testChain mirrors the SpaceTraders supply-chain export→imports shape:
// export X has imports = the goods needed to produce X.
func testChain() map[string][]string {
	return map[string][]string{
		"FAB_MATS":   {"IRON", "QUARTZ_SAND"},
		"IRON":       {"IRON_ORE"},
		"SHIP_PARTS": {"EXPLOSIVES", "IRON"},
		"EXPLOSIVES": {"AMMONIA_ICE"},
	}
}

func childNames(n *chainNode) []string {
	names := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		names = append(names, c.Name)
	}
	return names
}

func TestBuildNeedsTree_FabMats(t *testing.T) {
	root := buildNeedsTree(testChain(), "FAB_MATS")
	if root.Name != "FAB_MATS" {
		t.Fatalf("root = %q, want FAB_MATS", root.Name)
	}
	if got, want := childNames(root), []string{"IRON", "QUARTZ_SAND"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	iron := root.Children[0]
	if got, want := childNames(iron), []string{"IRON_ORE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IRON children = %v, want %v", got, want)
	}
	if len(iron.Children[0].Children) != 0 {
		t.Fatalf("IRON_ORE should be a leaf, has %d children", len(iron.Children[0].Children))
	}
	if len(root.Children[1].Children) != 0 {
		t.Fatalf("QUARTZ_SAND should be a leaf, has %d children", len(root.Children[1].Children))
	}
}

func TestBuildNeedsTree_ExplosivesLeaf(t *testing.T) {
	root := buildNeedsTree(testChain(), "SHIP_PARTS")
	if got, want := childNames(root), []string{"EXPLOSIVES", "IRON"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	if len(root.Children[0].Children) != 0 {
		t.Fatalf("EXPLOSIVES must not be expanded, has %d children", len(root.Children[0].Children))
	}

	expRoot := buildNeedsTree(testChain(), "EXPLOSIVES")
	if expRoot.Name != "EXPLOSIVES" || len(expRoot.Children) != 0 {
		t.Fatalf("EXPLOSIVES as root must stay a single node, got %+v", expRoot)
	}
}

func TestBuildNeedsTree_CycleGuard(t *testing.T) {
	cycle := map[string][]string{"A": {"B"}, "B": {"A"}}
	root := buildNeedsTree(cycle, "A")
	if got, want := childNames(root), []string{"B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	b := root.Children[0]
	if got, want := childNames(b), []string{"A"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("B children = %v, want %v (cycle shown once, not expanded)", got, want)
	}
	if len(b.Children[0].Children) != 0 {
		t.Fatal("cycle node A must be a leaf")
	}
}

func TestBuildNeedsTree_UnknownGood(t *testing.T) {
	root := buildNeedsTree(testChain(), "WATER_ICE")
	if root.Name != "WATER_ICE" {
		t.Fatalf("root = %q, want WATER_ICE", root.Name)
	}
	if len(root.Children) != 0 {
		t.Fatalf("unknown good should be a leaf, has %d children", len(root.Children))
	}
}

func TestBuildNeedsTree_EmptyChain(t *testing.T) {
	root := buildNeedsTree(map[string][]string{}, "FAB_MATS")
	if root.Name != "FAB_MATS" || len(root.Children) != 0 {
		t.Fatalf("empty chain should yield a single FAB_MATS node, got %+v", root)
	}
}

func TestChainGoods_Needs(t *testing.T) {
	want := []string{"EXPLOSIVES", "FAB_MATS", "IRON", "SHIP_PARTS"}
	if got := chainGoods(testChain(), "needs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("chainGoods(needs) = %v, want %v", got, want)
	}
	if got := chainGoods(testChain(), "bogus"); !reflect.DeepEqual(got, want) {
		t.Fatalf("chainGoods(bogus) = %v, want default %v", got, want)
	}
	if got := chainGoods(map[string][]string{}, "needs"); got == nil || len(got) != 0 {
		t.Fatalf("chainGoods(empty) = %v, want empty non-nil", got)
	}
}

func TestBuildTreeData(t *testing.T) {
	d := buildTreeData(buildNeedsTree(testChain(), "FAB_MATS"))
	if d.Name != "FAB_MATS" {
		t.Fatalf("Name = %q, want FAB_MATS", d.Name)
	}
	if len(d.Children) != 2 {
		t.Fatalf("len(Children) = %d, want 2", len(d.Children))
	}
	if got := d.Children[0].Children[0].Name; got != "IRON_ORE" {
		t.Fatalf("grandchild = %q, want IRON_ORE", got)
	}
}

func TestSupplyChainChart_SnippetPins(t *testing.T) {
	root := buildNeedsTree(testChain(), "FAB_MATS")
	snippet := SupplyChainChart(root, "Requirements for FAB_MATS", "RL").RenderSnippet()
	for _, want := range []string{`"orient":"RL"`, `"roam":true`, `"initialTreeDepth":-1`, "IRON_ORE"} {
		if !strings.Contains(snippet.Script, want) {
			t.Errorf("snippet missing %q", want)
		}
	}
}
