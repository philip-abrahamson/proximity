// Copyright Philip Abrahamson 2025-2026
// Copyright High Country Software Ltd 2002-2004
//
// Licensed under the GNU General Public License version 2.0 (GPLv2)

package geodata

import (
	"bufio"
	"cmp"
    "encoding/csv"
    "fmt"
	"io"
	"log"
	"math"
    "os"
	"slices"
	"strconv"
)

const Debug = true

// Geospatial index consisting of the location along a fractal, space-filling curve.
// Named after the discoverer, 19th century Italian mathematician Giuseppe Peano
type Peano uint32

// Number of bits of information to retain in the peano code
// i.e. its level of digitisation.
// We started with 16 bits, but that provides a resolution of
// about 600m, (diameter of world ~40,000km / 2**16) which might not suit all applications.
// 19 bits would be under 100m.
// However, the larger the number of bits, the longer the PeanoIndex
// e.g. 19 bits will lead to 524,288 array elements
// per index, whereas 16bits is only 65,536 elements
// IF CHANGING THIS - you must also manually change PeanoIndex (index.go) to use a size of 2**PeanoBits
// and use uint32 instead of uint16 when casting ints...
// SEE ALSO CalcPeano() which has this hardcoded currently...
const PeanoBits = 16

// Each Record includes:
// Title, Description for free text data
// URL, an optional hyperlink
// Bitmap, a 64 bit integer bitmap designed to store up to 64 binary flags
//   indicating different properties of each record.
//   For example, a real-estate search application might use several flags to
//   indicate different price ranges of properties, and other flags to indicate
//   numbers of bedrooms, garages, etc.  The mapping of the flags and their
//   interpretation is the responsibility of the developer using this proximity engine.
// Lat, Lon are latitude and longitude, traditional geospatial coordinates
// Peano1 and Peano2, less conventional geospatial indices consisting of the
//   location along a fractal, space-filling curve.
//   We use "Peano codes" to estimate proximity, because it is far more
//   performant looking along a one-dimensional indexed integer to find data
//   than making a two-dimensional geospatial query.
//   We use two Peano codes to improve average accuracy, because each Peano code
//   by itself has quite variable accuracy.
type Record struct {
	// only capitalised field names are properly converted to JSON
	ID string `json:"id" binding:"required,string"`
	Title string `json:"title"`
	Description string `json:"description"`
	URL string `json:"url"`
	Bitmap uint64 `json:"bitmap"`
	Lat float64 `json:"lat" binding:"required,float64"`
	Lon float64 `json:"lon" binding:"required,float64"`
	Peano1 Peano
	Peano2 Peano
}

type ResultRecord struct {
	ID string `json:"id" binding:"required,string"`
	Title string `json:"title"`
	Description string `json:"description"`
	URL string `json:"url"`
	Bitmap uint64 `json:"bitmap"`
	Lat float64 `json:"lat" binding:"required,float64"`
	Lon float64 `json:"lon" binding:"required,float64"`
	Distance float64 `json:"distance" binding:"required,float64"`
	Units string `json:"units" binding:"required,string"`
}

// Our geospatial data includes the following data structures:
//
// * "records"
//   A slice containing each data record (type Record)
//
// * "peanoIndex1", "peanoIndex2"
//   Two searchable indexes of "Peano codes" pointing at
//   the next peano codes in the series.
//   (Peano codes are fractal space-filling curves discovered by
//    19th century mathematician Giuseppe Peano. We use them to scale our
//    proximity queries.  We use two separate peano codes offset from
//    each other to minimise the spatial distortions inherent when using
//    a one-dimensional curve to describe a two-dimensional space)
//
//  * "peanoMap1", "peanoMap2"
//    maps of peano code to a slice containing pointers to data records
//    for each record having that same peano code location.
//
// What we do when we search is:
// 1. convert the input geospatial latitude & longitude coordinates
//    into our two Peano codes
// 2. look-up peanoIndex1 & peanoIndex2 to find the locations of
//    that peano and a set number of codes before and after it.
//    We will then look up each record in turn in the peanoMaps
//    and check each records' bitmap field matches any
//    boolean logic applied in the query.
//
// Currently this has a weakness if the query is for a very rare
// property in the bitmap, because the scan of each record could
// continue through the majority of all records before finding
// a matching record.
// We will need to limit the number of records searched
// in these cases, and cut off the search with a message like
// "no results for X found within a range of Y km/mi"
// What would help here is something like a count or other
// indication of the distribution of a query property among
// the records, and if this count were too low we could
// instead use an alternative index based on the direct
// properties instead of geospatial location, and then
// sort these by location to find the nearest.
type GeoData struct {
	records []Record
	peanoIndex1 *PeanoIndex
	peanoIndex2 *PeanoIndex
	peanoMap1 map[Peano][]*Record
	peanoMap2 map[Peano][]*Record
}

// Search results slice
type Results []ResultRecord

// CSV column positions of each field based on the header line
type HeaderPosition struct {
	ID int
	Title int
	Description int
	URL int
	Bitmap int
	Lat int
	Lon int
}

// Origin of secondary offset peano codes,
// chosen to avoid inherent issues with origin 0, 0 peano codes.
// The worst cases are the UK W/E at Greenwich, and the US N/S of
// Minneapolis (45 deg). We'll hence shift to the W by 30 deg which
// is over the Azores in the Atlantic Ocean, and to the North by 20deg
// which is to the S of Alaska.
// The secondary offset peano codes will have their own problematic
// locations, but the combination of the two sets of peano codes
// minimises these issues.
// Note: to ensure the grids don't naturally re-align nearby, it is
// useful to have some random noise added e.g. not exactly -20 or 30.
const OffsetLat = -23.7432
const OffsetLon = 29.3456

// bitmap fields are uint64
const BitmapSize = 64
// lat/lon fields are float64
const LatLonSize = 64

const KmPerDegree = 111.195
const MilesPerDegree = 69.094

// Import a CSV file at the input path
// and generate our proximity data in-memory
func (geo *GeoData) Import(path string) error {
	fh, errOpen := os.Open(path)
	if errOpen != nil {
		return fmt.Errorf("Failed to open CSV file '%s' - %s", path, errOpen.Error())
	}
	defer fh.Close()

	buffer := bufio.NewReader(fh)
	reader := csv.NewReader(buffer)
	cnt := 1
	var headerPos HeaderPosition
	for {
		line, err := reader.Read()
		// exit after the last line
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		err = geo.ImportLine(&headerPos, line, cnt)
		if err != nil {
			return err
		}

		cnt++
	}

	geo.PopulateIndexes()

	return nil
}

// PopulateIndexes: Populate the Peano binary search indexes & maps
func (geo *GeoData) PopulateIndexes() {
	geo.peanoIndex1 = NewPeanoIndex()
	geo.peanoIndex2 = NewPeanoIndex()

	if Debug {
		log.Printf("Generating binary search index for %d records...\n", len(geo.records))
	}

	geo.peanoMap1 = make(map[Peano][]*Record)
	geo.peanoMap2 = make(map[Peano][]*Record)

	for _, v := range geo.records {
		peano1 := v.Peano1
		peano2 := v.Peano2
		_, exists1 := geo.peanoMap1[peano1]
		_, exists2 := geo.peanoMap2[peano2]
		if exists1 {
			geo.peanoMap1[peano1] = append(geo.peanoMap1[peano1], &v)
		} else {
			geo.peanoMap1[peano1] = []*Record{&v,}
			geo.peanoIndex1.ReplaceOrInsert(peano1)
		}
		if exists2 {
			geo.peanoMap2[peano2] = append(geo.peanoMap2[peano2], &v)
		} else {
			geo.peanoMap2[peano2] = []*Record{&v,}
			geo.peanoIndex2.ReplaceOrInsert(peano2)
		}
	}

	geo.peanoIndex1.Process()
	geo.peanoIndex2.Process()

	return
}

func (geo *GeoData) ImportLine (hp *HeaderPosition, line []string, cnt int) (err error) {

	// handle the header line by storing the header positions
	if cnt == 1 {
		storeHeaders(hp, line)
		return
	}

	// import a data line
	// but first check we have seen some headers
	if hp == nil {
		panic("No headers line found in this CSV file!")
	}

	bmap, errBmap := strconv.ParseUint(line[hp.Bitmap], 0, BitmapSize)
	if errBmap != nil {
		return fmt.Errorf("On line %d failed to parse bitmap '%s' - %s", cnt, line[hp.Bitmap], errBmap)
	}
	lat, errLat := strconv.ParseFloat(line[hp.Lat], LatLonSize)
	if errLat != nil {
		return fmt.Errorf("On line %d failed to parse lat '%s' - %s", cnt, line[hp.Lat], errBmap)
	}
	if lat > 90 || lat < -90 {
		return fmt.Errorf("On line %d lat '%s' outside range -90 to +90", cnt, line[hp.Lat])
	}

	lon, errLon := strconv.ParseFloat(line[hp.Lon], LatLonSize)
	if errLon != nil {
		return fmt.Errorf("On line %d failed to parse lon '%s' - %s", cnt, line[hp.Lon], errBmap)
	}
	if lon > 180 || lon < -180 {
		return fmt.Errorf("On line %d lat '%s' outside range -180 to +180", cnt, line[hp.Lat])
	}

	newR := Record{
		Title: line[hp.Title],
		Description: line[hp.Description],
		URL: line[hp.URL],
		Bitmap: bmap,
		Lat: lat,
		Lon: lon,
	}
	if line[hp.ID] != "" {
		newR.ID = line[hp.ID]
	} else {
		newR.ID = fmt.Sprintf("%d", cnt)
	}

	newR.Peano1 = CalcPeano(lat, lon)
	newR.Peano2 = CalcPeanoOffset(lat, lon)

	geo.records = append(geo.records, newR)

	return
}

// Search the geodata for matching records
func (geo *GeoData) Find(lat, lon float64, bitmask uint64, max uint64, units string) []ResultRecord {

	// final results to return
	var res []ResultRecord
	// intermediate slice of records to sort & potentially limit before becoming results
	var recs []Record

	uniqueRecords := make(map[*Record]bool)

	// Don't go past the number of results desired when
	// walking along either peano curve in either direction
	var maxResUp1, maxResUp2, maxResDown1, maxResDown2 int
	intMax := int(max)
	maxResUp1 = intMax
	maxResUp2 = intMax
	maxResDown1 = intMax
	maxResDown2 = intMax

	// Don't keep trying to obtain results indefinitely
	var maxAt, maxAttemptsUp1, maxAttemptsUp2, maxAttemptsDown1, maxAttemptsDown2 int
	maxAt = int(max * 4)
	maxAttemptsUp1 = maxAt
	maxAttemptsUp2 = maxAt
	maxAttemptsDown1 = maxAt
	maxAttemptsDown2 = maxAt

	if units != "mi" {
		units = "km"
	}

	// obtain our Peano & offset Peano codes for our input coords
	peano1 := CalcPeano(lat, lon)
	peano2 := CalcPeanoOffset(lat, lon)

	// find the locations of the first record matching
	// these peanos in the peanoIndex
	iterator := func(peano Peano, maxAttempts *int, maxRes *int, pMap map[Peano][]*Record) bool {

		// Cut out in case there are no matching results
		*maxAttempts--
		if *maxAttempts < 0 {
			return false
		}
		candidates, exists := pMap[peano]
		if ! exists {
			// e.g. a peano generated by subtracting one from an existing one
			return true
		}
		for i := 0; i < len(candidates); i++ {
			rec := candidates[i]
			if _, exists := uniqueRecords[rec]; exists {
				continue
			}
			// check each record matches the bitmask, if provided
			if bitmask > 0 {
				// Assume A AND B AND C ... for the bitmask
				// we will add more boolean logic later...
				if rec.Bitmap & bitmask != bitmask {
					// the AND logic failed, so return early
					// but continue iterating (true)
					return true
				}
			}
			// cut out if we've hit the maximum desired results
			*maxRes--
			if *maxRes < 0 {
				return false
			}
			// add the record to our intermediate slice of records
			recs = append(recs, *rec)
			uniqueRecords[rec] = true
		}
		return true
	}

	iteratorUp1 := func(p Peano, first bool) bool {
		return iterator(p, &maxAttemptsUp1, &maxResUp1, geo.peanoMap1)
	}
	iteratorDown1 := func(p Peano, first bool) bool {
		return iterator(p, &maxAttemptsDown1, &maxResDown1, geo.peanoMap1)
	}
	iteratorUp2 := func(p Peano, first bool) bool {
		return iterator(p, &maxAttemptsUp2, &maxResUp2, geo.peanoMap2)
	}
	iteratorDown2 := func(p Peano, first bool) bool {
		return iterator(p, &maxAttemptsDown2, &maxResDown2, geo.peanoMap2)
	}

	// traverse each index up and down and merge the results into recs
	geo.peanoIndex1.AscendGreaterOrEqual(peano1, iteratorUp1)
	if (peano1 > 0) {
		// subtract 1 to avoid duplicating that peano
		geo.peanoIndex1.DescendLessOrEqual(peano1 - 1, iteratorDown1)
	}
	geo.peanoIndex2.AscendGreaterOrEqual(peano2, iteratorUp2)
	if (peano2 > 0) {
		// subtract 1 to avoid duplicating that peano
		geo.peanoIndex2.DescendLessOrEqual(peano2 - 1, iteratorDown2)
	}

	// Sort by proximity before cutting down to the expected result count.
	// One option here might be to use a fake proximity e.g. (abs(x) + abs(y))
	// instead of the accurate (x*x) + (y*y) (we don't need to take a square
	// root when comparing proximities while sorting)
	// but because we might only be expecting to get e.g. 20 records in total
	// there would only be 80 records at most to filter, (20 per space curve
	// in two directions) and these two different equations won't result in
	// a significant performance difference for such a small number of
	// calculations.
	// Perhaps if a larger number of results were being returned it might
	// be worthwhile?
	recProx := map[*Record]float64{}
	for _, rec := range recs {
		deltaLat := lat - rec.Lat
		recProx[&rec] = proximityForSort(deltaLat/2, deltaLat, lon - rec.Lon)
	}
	slices.SortFunc(recs, func(a, b Record) int {
		proxA, _ := recProx[&a]
		proxB, _ := recProx[&b]
		return cmp.Compare(proxA, proxB)
	})

	// Cut down the results by slicing by either the smaller of the desired
	// max records or the count of the current results
	var maxLen uint64
	maxLen = uint64(len(recs))
	if max < maxLen {
		maxLen = max
	}
	for _, rec := range recs[:maxLen] {
		rrec := ResultRecord{
			ID: rec.ID,
			Title: rec.Title,
			Description: rec.Description,
			URL: rec.URL,
			Bitmap: rec.Bitmap,
			Lat: rec.Lat,
			Lon: rec.Lon,
			Distance: proximity(recProx[&rec], units),
			Units: units,
		}

		res = append(res, rrec)
	}

	return res
}

func storeHeaders(hp *HeaderPosition, line []string) {
	for i, v := range line {
		switch v {
		case "ID":
			hp.ID = i
		case "Title":
			hp.Title = i
		case "Description":
			hp.Description = i
		case "URL":
			hp.URL = i
		case "Bitmap":
			hp.Bitmap = i
		case "Lat":
			hp.Lat = i
		case "Lon":
			hp.Lon = i
		default:
			panic(fmt.Sprintf("header field '%s' not recognised!", v))
		}
	}
	return
}

// Create two peano codes from a floating point latitude/longitude
// value on the earth's surface. Assume a spherical projection
// where 1.0 latitude = 1.0 longitude (although in reality
// the earth is closer to an ellipsoid).
func CalcPeano(lat, lon float64) Peano {

	// TODO - use PeanoBits to generalise this instead of assuming 16bits
	lat16, lon16 := digitiseDegrees(lat, lon)

	var maskIn uint16
	var maskOut uint32

	// Interleave the bits from a latitude value with the bits
	// from a longitude value.

	// start with an int so we can perform maths
	// and cast to a Peano on output
	var peano uint32
	peano = 0
	maskIn = 1
	maskOut = 2

	// TODO - use PeanoBits to generalise this instead of assuming 16bits
	for i := 0; i < 16; i++ {

		if (lat16 & maskIn) != 0 {
			peano += maskOut
		}

		maskIn = maskIn << 1
		maskOut = maskOut << 2
	}

	maskIn = 1
	maskOut = 1
	// TODO - use PeanoBits to generalise this instead of assuming 16bits
	for i := 0; i < 16; i++ {

		if (lon16 & maskIn) != 0 {
			peano += maskOut
		}

		maskIn = maskIn << 1
		maskOut = maskOut << 2
	}

	return Peano(peano)
}

func CalcPeanoOffset(lat, lon float64) (peano Peano) {
	// calculate an offset geo coordinate for our second peano code
	latOffset, lonOffset := Offset(lat, lon)
	return CalcPeano(latOffset, lonOffset)
}

func digitiseDegrees(lat, lon float64) (lat16, lon16 uint16) {
	// Convert the lat/lon into 16 bit ints
	// centered on the equator (ie. 32768=Equator)
	// and the 0 = -180deg, 65536 = +180deg
	lat16 = uint16(((lat + 90.0)/180.0 * 32767) + 16384)
	lon16 = uint16((lon + 180.0)/360.0 * 65535)
	return lat16, lon16
}

// Offset the input lat/lon degrees by a particular
// distance in lat and lon. This will ensure two approximations
// to the nearest points can be joined together to form
// one good approximation, removing the chance of being near the
// edge of a larger quad-tree boundary.
func Offset(lat, lon float64) (latOff, lonOff float64) {

	// Offset the coordinates
	latOff = lat + OffsetLat
	lonOff = lon + OffsetLon

	// Wrap to the other side of the world horizontally
	// (not needed vertically because still inside the peano's square
	// which extends to 360*360 degs)
	if lonOff < -180.0 {
		lonOff = lonOff + 360.0
	}
	if lonOff > 180.0 {
		lonOff = lonOff - 360.0
	}

	return latOff, lonOff
}

// Cosine table - used to estimate the cosine of latitude values
// which are used to scale the distance across the earth in
// a longitudinal direction.
// E.g. as you get closer to a pole, the actual distance
// of a degree of longitude starts to decrease towards zero
// x = cos(lat) * lon * C (constant based on size of earth)
// We use only positive latitudes to save space, should we
// increase the size of this table.
var cosineTable map[int]float64
func cosineEstimate(latInt int) float64 {
	if cosineTable == nil {
		generateCosineTable()
	}
	// sign isn't important for cosines
	if latInt < 0 {
		latInt = -latInt
	}
	if latInt > 90 {
		log.Printf("latitude %d > 90 - this should be impossible!", latInt)
		return 0 // cos 90 == 0
	}
	return cosineTable[latInt]
}

func generateCosineTable() {
	cosineTable = make(map[int]float64)
	for deg := 0; deg <= 90; deg++ {
		rad := float64(deg) * math.Pi / 180.0
		cosineTable[deg] = math.Cos(rad)
	}
	return
}

// Estimate of the square of the proximity for sorting purposes.
// This should only be used on a subset of results.
// D stands for delta between the search latitude & a result latitude
func proximityForSort(meanLat float64, latD float64, lonD float64) float64 {
	cosLonEstimate := cosineEstimate(int(meanLat))
	// Note: a less accurate, but faster method is:
	// 3/8 * (latD + (lonD * cosLonEstimate))
	return (latD * latD) + (lonD * cosLonEstimate * lonD * cosLonEstimate)
}

func proximity(proxForSort float64, units string) float64 {
	proxDegrees := math.Sqrt(proxForSort)
	if units == "mi" {
		return proxDegrees * MilesPerDegree
	}
	return proxDegrees * KmPerDegree
}

// type bspTree
