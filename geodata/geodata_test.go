package geodata

import (
	"testing"
	"fmt"
	"strconv"
	"time"
)

// TestSpiral: populate search data using a spiral starting at lat 0, lon 0
// then search at the origin of the spiral for the first 20 records in the spiral
// 0.0001 is approx 10m at the inner arm of the spiral - which will feature many duplicated peanos
func TestSpiral(t *testing.T) {
	recCnt := 360000
	start := time.Now()
	geo := PopulateData(0.0, 0.0, 0.001, recCnt)
	// geo := PopulateData(0.0, 0.0, 0.0001, recCnt)
	t.Logf("proximity data population of %d records took %s", recCnt, time.Since(start))
	var expect int
	expect = 20
	fstart := time.Now()
	res := geo.Find(float64(0), float64(0), uint64(0), uint64(expect), "km")
	t.Logf("proximity data search of %d records for %d results took %s", recCnt, expect, time.Since(fstart))
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
			line = []string{fmt.Sprintf("%d", cnt), fmt.Sprintf("Title %d", cnt), fmt.Sprintf("Description %d", cnt), fmt.Sprintf("https://test.com/%d", cnt), "0", fmt.Sprintf("%0.6f", lat), fmt.Sprintf("%0.6f", lon)}
		}
		err := geo.ImportLine(&headerPos, line, i)
		if err != nil {
			panic(err)
		}
	}
	geo.PopulateIndexes()
	return geo
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
	if lat > 90 {
		lat -= 180
	}
	if lat < -90 {
		lat += 180
	}
	if lon > 180 {
		lon -= 360
	}
	if lon < -180 {
		lon += 360
	}
	return bearing, lat, lon
}
