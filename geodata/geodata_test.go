// Copyright Philip Abrahamson 2025-2026
// Copyright High Country Software Ltd 2002-2004
//
// Licensed under the GNU General Public License version 2.0 (GPLv2)

package geodata

import (
	"testing"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TestSpiral: populate search data using a spiral starting at lat 0, lon 0
// then search at the origin of the spiral for the first 20 records in the spiral
// 0.0001 is approx 10m at the inner arm of the spiral - which will feature many duplicated peanos
func TestSpiral(t *testing.T) {
	// recCnt := 1000000
	// recCnt := 360000
	// recCnt := 200
	recCnt := 100
	// recCnt := 40
	// recCnt := 20
	start := time.Now()
	geo := PopulateData(0.0, 0.0, 0.01, recCnt)
	// geo := PopulateData(0.0, 0.0, 0.0001, recCnt)
	t.Logf("proximity data population of %d records took %s", recCnt, time.Since(start))
	var expect int
	expect = 20
	fstart := time.Now()
	res := geo.Find(float64(0), float64(0), uint64(0), uint64(expect), "km")
	t.Logf("proximity data search of %d records for %d results took %s", recCnt, expect, time.Since(fstart))
	uniqueIds := make(map[string]bool)
	ids := []string{}
	for _, r := range res {
		uniqueIds[r.ID] = true
		ids = append(ids, r.ID)
	}
	t.Logf("IDs returned: %s\n", strings.Join(ids, ", "))
	if len(res) != len(uniqueIds) {
		t.Errorf("Got some non-unique ids in the results!")
	}
	if len(res) != expect {
		t.Errorf("Got %d results instead of %d results", len(res), expect)
	}
	cnt := 0
	for _, rec := range res {
		id, err := strconv.Atoi(rec.ID)
		if err != nil {
			panic(err)
		}
		if id <= expect  {
			cnt++
		}
	}
	// We don't test for 100% here, because the results could potentially be erratic.
	// Our system is designed for speed over accuracy.
	t.Logf("Got %.0f%% of the results we expected\n", float64(100) * float64(cnt)/float64(expect))

	// try another search at another location
	res2 := geo.Find(float64(50), float64(1.12345), uint64(0), uint64(expect), "km")
	ids = []string{}
	for _, r := range res2 {
		ids = append(ids, r.ID)
	}
	t.Logf("Second search at a different location, IDs returned: %s\n", strings.Join(ids, ", "))

	// benchmark the search a little by traversing the globe
	benchCnt := 0
	t0 := time.Now()
	for lat := -90; lat <= 90; lat++ {
		for lon := -180; lon <= 180; lon++ {
			_ = geo.Find(float64(lat), float64(lon), uint64(0), uint64(expect), "km")
			benchCnt++
		}
	}
	tD := time.Since(t0)
	t.Logf("Performed %d searches for %d records each time, which took a total time %v at an average time per search of %0.fµs", benchCnt, expect, tD, float64(int64(tD)/int64(benchCnt))/1000)
}

// TestPeano is just a "sight" test
func TestPeano(t *testing.T) {
	peano := CalcPeano(50.123456, 0.123456)
	t.Logf("Peano code generated was: %v", peano)
}

func PopulateData(lat float64, lon float64, delta float64, count int) *GeoData {
	geo := new(GeoData)
	var headerPos HeaderPosition
	bearing := 'N'
	// 1 is for the header line
	for i := 1; i <= count + 1; i++ {
		cnt := i - 1
		var line []string
		if i == 1 {
			line = []string{"ID", "Title", "Description", "URL", "Bitmap", "Lat", "Lon"}
		} else {
			bearing, lat, lon = Spiral(bearing, lat, lon, delta, cnt)
			line = []string{fmt.Sprintf("%d", cnt), fmt.Sprintf("Title %d", cnt), fmt.Sprintf("Description %d", cnt), fmt.Sprintf("https://test.com/%d", cnt), fmt.Sprintf("%d",cnt), fmt.Sprintf("%0.6f", lat), fmt.Sprintf("%0.6f", lon)}
		}
		err := geo.ImportLine(&headerPos, line, i)
		if err != nil {
			panic(err)
		}
	}
	geo.PopulateIndexes("test")
	return geo
}

func TestLogic(t *testing.T) {
	expect := 2
	geo := PopulateData(0.0, 0.0, 0.0001, expect)
	res0 := geo.Find(float64(0), float64(0), uint64(0), uint64(expect), "km")
	if len(res0) != 2 {
		t.Errorf("Failed to get all records with a 0 bitmask")
	}
	res1 := geo.Find(float64(0), float64(0), uint64(1), uint64(expect), "km")
	if len(res1) != 1 || res1[0].ID != "1" {
		t.Errorf("Failed to get only the first record with a 1 bitmask")
	}
	res2 := geo.Find(float64(0), float64(0), uint64(2), uint64(expect), "km")
	if len(res2) != 1 || res2[0].ID != "2" {
		t.Errorf("Failed to get only the second record with a 2 bitmask")
	}
}

func Spiral(bearing rune, lat, lon, delta float64, i int) (rune, float64, float64) {
	// The distance of each arm of the spiral follows an incrementing pattern
	// 1, 1, 2, 2, 3, 3, ...
	// where each above number is also multiplied by an input delta
	arm := delta * float64(int((i + 1)/2))
	if bearing == 'N' {
		lon -= arm
		bearing = 'W'
	} else if bearing == 'W' {
		lat -= arm
		bearing = 'S'
	} else if bearing == 'S' {
		lon += arm
		bearing = 'E'
	} else if bearing == 'E' {
		lat += arm
		bearing = 'N'
	}
	if lat > 90 || lat < -90 {
		lat = 0 // reset spiral
	}
	if lon > 180 || lon < -180 {
		lon = 0 // reset spiral
	}
	return bearing, lat, lon
}
