package store

import "testing"

// located builds a node carrying coordinates. Names are generic — these tests
// pin geometry, not any particular mesh.
func located(name string, lat, lon float64) Node {
	return Node{PublicKey: name, Name: name, Latitude: &lat, Longitude: &lon, HasLocation: true}
}

// mainCluster is a dense metro population: 40 nodes inside about half a degree,
// the shape that makes an IQR spread tiny.
func mainCluster() []Node {
	var out []Node
	for i := 0; i < 40; i++ {
		lat := 45.0 + float64(i%8)*0.05
		lon := -120.0 - float64(i%5)*0.06
		out = append(out, located("main", lat, lon))
	}
	return out
}

func suspectByName(t *testing.T, nodes []Node, name string) bool {
	t.Helper()
	for i := range nodes {
		if nodes[i].Name == name {
			return nodes[i].GpsSuspect
		}
	}
	t.Fatalf("node %q not found", name)
	return false
}

// TestGpsOutliersKeepsRemoteCluster is the regression that prompted this code.
// A real regional group sitting a few hundred km outside the main cluster is
// coverage, not corrupt GPS, and must stay plottable. The previous rule tested
// latitude and longitude independently against 3×IQR whiskers of the whole
// population, so a node that was moderately north AND moderately west of a
// dense cluster failed both axes and was hidden from every map — one such node
// missed the latitude whisker by under a kilometre.
func TestGpsOutliersKeepsRemoteCluster(t *testing.T) {
	nodes := mainCluster()
	// ~330 km north-west of the cluster centroid, spread over ~30 km.
	nodes = append(nodes,
		located("remote-a", 46.4, -124.2),
		located("remote-b", 46.5, -124.0),
		located("remote-c", 46.6, -124.1),
	)

	flagGpsOutliers(nodes)

	for _, name := range []string{"remote-a", "remote-b", "remote-c"} {
		if suspectByName(t, nodes, name) {
			d := haversineKm(45.0, -120.0, *nodes[len(nodes)-1].Latitude, *nodes[len(nodes)-1].Longitude)
			t.Errorf("%s flagged as suspect; a cluster ~%.0f km out is inside the %d km floor", name, d, gpsFloorKm)
		}
	}
}

// TestGpsOutliersFlagsNullIsland covers the actual corrupt-GPS case: a node
// that has never had a fix reports 0,0. It is invalid structurally, so it is
// flagged regardless of how the rest of the population is distributed.
func TestGpsOutliersFlagsNullIsland(t *testing.T) {
	nodes := append(mainCluster(), located("no-fix", 0, 0))

	flagGpsOutliers(nodes)

	if !suspectByName(t, nodes, "no-fix") {
		t.Error("node at 0,0 not flagged as suspect")
	}
}

// TestGpsOutliersIgnoreInvalidWhenJudgingOthers pins the second half of the
// null-island fix: 0,0 rows must not participate in the centroid or the
// whisker. Otherwise whether a real remote node is visible depends on how many
// broken nodes happen to be on air — and cleaning up the broken ones would
// silently hide the real ones.
func TestGpsOutliersIgnoreInvalidWhenJudgingOthers(t *testing.T) {
	build := func(zeros int) []Node {
		n := append(mainCluster(), located("remote", 46.5, -124.0))
		for i := 0; i < zeros; i++ {
			n = append(n, located("no-fix", 0, 0))
		}
		return n
	}

	none, many := build(0), build(20)
	flagGpsOutliers(none)
	flagGpsOutliers(many)

	if got, want := suspectByName(t, many, "remote"), suspectByName(t, none, "remote"); got != want {
		t.Errorf("verdict for the remote node changed with 20 null-island rows present: %v vs %v", got, want)
	}
	if suspectByName(t, none, "remote") {
		t.Error("remote node flagged with no null-island rows present")
	}
}

// TestGpsOutliersFlagsDistantOutlier keeps the detector useful: coordinates
// that are structurally valid but nowhere near the mesh are still suspect.
func TestGpsOutliersFlagsDistantOutlier(t *testing.T) {
	nodes := append(mainCluster(), located("other-continent", 12.0, 34.0))

	flagGpsOutliers(nodes)

	if !suspectByName(t, nodes, "other-continent") {
		t.Error("node thousands of km from the mesh not flagged as suspect")
	}
}

// TestGpsOutliersSkipUnlocated — a node with no coordinates at all is not a GPS
// error, and must never be marked suspect.
func TestGpsOutliersSkipUnlocated(t *testing.T) {
	nodes := append(mainCluster(), Node{PublicKey: "unlocated", Name: "unlocated"})

	flagGpsOutliers(nodes)

	if suspectByName(t, nodes, "unlocated") {
		t.Error("node with no coordinates flagged as suspect")
	}
}

// TestGpsOutliersSmallPopulation — under 8 located nodes there is no
// distribution to judge against, so only invalid coordinates are flagged.
func TestGpsOutliersSmallPopulation(t *testing.T) {
	nodes := []Node{
		located("a", 45.0, -120.0),
		located("far", 12.0, 34.0),
		located("no-fix", 0, 0),
	}

	flagGpsOutliers(nodes)

	if suspectByName(t, nodes, "far") {
		t.Error("distance outlier flagged from a population too small to judge")
	}
	if !suspectByName(t, nodes, "no-fix") {
		t.Error("0,0 must be flagged even in a small population")
	}
}
