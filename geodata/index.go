package geodata

import (
	"cmp"
	"math"
	"slices"
)

// PeanoIndex loosely follows the interface of llrb
// "github.com/petar/GoLLRB/llrb" which we originally
// intended to use here, but discovered it was too slow
// for our purposes.
// PeanoIndex, unlike llrb, is currently a write-once
// data structure.  It requires a call to Process()
// before use.
type PeanoIndex struct {
	// Peanos is a sorted slice of peano codes - points on a fractal space filling curve
	Peanos []Peano
	// Links is effectively a linked list pointing at the slice index
	// of the previous peano and next peano in the Peanos slice,
	// to make moving forward and backwards along a peano curve
	// much faster. Each map points at an array of
	// [previous index, next index]
	Links map[Peano][2]int
	// Ranges stores the max and min slice index over a particular range
	// being the high 16bits of the peano code,
	// which could cut down the binary search space
	// to no more than 2**16 codes.
	Ranges map[uint16][2]int
}

var maxPeano = uint32(math.Pow(2, 32) - 1)
var minPeano = 0
const max16bit = 65536

// NewPeanoIndex returns a pointer to
// a new PeanoIndex struct.
func NewPeanoIndex() *PeanoIndex {
	pi := PeanoIndex{}
	return &pi
}

// ReplaceOrInsert inserts a new peano code
// into the index, but note that it won't be
// searchable until Process() is run.
func (pi *PeanoIndex) ReplaceOrInsert(p Peano) {
	pi.Peanos = append(pi.Peanos, p)
}

// Process creates the "indexed linked-list" data structure
// by creating an index link between the elements
// already marked with 1's by ReplaceOrInsert().
func (pi *PeanoIndex) Process() {

	// sort the peanos
	slices.SortFunc(pi.Peanos, func(a, b Peano) int {
		return cmp.Compare(uint32(a), uint32(b))
	})

	// populate the Links & Ranges
	pi.Links = make(map[Peano][2]int)
	pi.Ranges = make(map[uint16][2]int)

	imax := len(pi.Peanos) - 1

	// if we have only one peano (or zero), there won't be any links or ranges
	if imax <= 0 {
		return
	}

	// populate the first and last links which wrap around
	// TODO - this might be a mistake, introducing subtle issues
	// test this doesn't infinitely loop the binarySearch for instance!
	pi.Links[ pi.Peanos[0] ] = [2]int{imax, 1}
	pi.Links[ pi.Peanos[imax] ] = [2]int{imax - 1, 0}

	for i, peano := range pi.Peanos {
		if i > 0 && i < imax {
			pi.Links[peano] = [2]int{i - 1, i + 1}
		}
		high16 := highBits(peano)
		minmax, exists := pi.Ranges[high16]
		if exists {
			if int(peano) < minmax[0] {
				minmax[0] = i
			}
			if int(peano) > minmax[1] {
				minmax[1] = i
			}
		} else {
			pi.Ranges[high16] = [2]int{i, i}
		}
	}

	return
}

// AscendLessOrEqual will search for the input peano 'p', and whether it finds
// it or not will then ascend up the peano curve and find the next peano
// codes and feed them one by one into the 'iterator' function passed in.
// The iterator function must return false at some point when enough
// results have been collected.
// 'first' is just a boolean flag to indicate whether this is the first
// or subsequent call, which helps us optimise the finding of peano codes.
func (pi *PeanoIndex) AscendGreaterOrEqual(p Peano, first bool, iterator func(p Peano, first bool) bool) {
	pi.ascendGreaterOrEqual(p, first, iterator)
}

// recursive function which exits when the iterator function returns false
func (pi *PeanoIndex) ascendGreaterOrEqual(p Peano, first bool, iterator func(p Peano, first bool) bool) bool {
	var nextPeano Peano
	if first {
		// Perform our binary search
		// but first narrow the range
		iMin, iMax := pi.rangeSearch(p)
		result := pi.binarySearch(p, iMin, iMax)
		if result.found {
			nextPeano = pi.Peanos[result.peanoIndex]
		} else {
			nextPeano = pi.Peanos[result.nextIndex]
		}
		first = false
	} else {
		// we already performed a binary search
		// so we can just follow the links upwards
		links, _ := pi.Links[p]
		nextPeano = pi.Peanos[links[1]]
	}
	// base of our recursion
	if !iterator(nextPeano, first) {
		return false
	}
	// recurse into this same function
	return pi.ascendGreaterOrEqual(nextPeano, first, iterator)
}

// DescendLessOrEqual will search for the input peano 'p', and whether it finds
// it or not will then descend down the peano curve and find the next peano
// codes and feed them one by one into the 'iterator' function passed in.
// The iterator function must return false at some point when enough
// results have been collected.
// 'first' is just a boolean flag to indicate whether this is the first
// or subsequent call, which helps us optimise the finding of peano codes.
func (pi *PeanoIndex) DescendLessOrEqual(p Peano, first bool, iterator func(p Peano, first bool) bool) {
	pi.descendLessOrEqual(p, first, iterator)
}

// descendLessOrEqual is a recursive function which exits when the iterator function returns false
func (pi *PeanoIndex) descendLessOrEqual(p Peano, first bool, iterator func(p Peano, first bool) bool) bool {
	var prevPeano Peano
	if first {
		// Perform our binary search
		// but first narrow the range
		iMin, iMax := pi.rangeSearch(p)
		result := pi.binarySearch(p, iMin, iMax)
		if result.found {
			prevPeano = pi.Peanos[result.peanoIndex]
		} else {
			prevPeano = pi.Peanos[result.prevIndex]
		}
		first = false
	} else {
		// we already performed a binary search
		// so we can just follow the links downwards
		links, _ := pi.Links[p]
		prevPeano = pi.Peanos[links[0]]
	}
	// base of our recursion
	if !iterator(prevPeano, first) {
		return false
	}
	// recurse into this same function
	return pi.descendLessOrEqual(prevPeano, first, iterator)
}

func (pi *PeanoIndex) rangeSearch(p Peano) (int, int) {
	high16 := highBits(p)
	irange, exists := pi.Ranges[high16]
	if ! exists {
		return 0, len(pi.Peanos) - 1
	}
	return irange[0], irange[1]
}

type binaryResults struct {
	found bool
	peanoIndex int
	prevIndex int
	nextIndex int
}

// binarySearch returns a struct of binaryResults
// which populates peanoIndex only if the peano is found
// and populates nextIndex and prevIndex in every case
func (pi *PeanoIndex) binarySearch(p Peano, minIndex int, maxIndex int) binaryResults {
	for {
		try := minIndex + int((maxIndex - minIndex) / 2)
		pTry := pi.Peanos[try]
		if pTry == p {
			// Found it! - look up the previous and next indexes
			links := pi.Links[pTry]
			res := binaryResults{
				found: true,
				peanoIndex: try,
				prevIndex: links[0],
				nextIndex: links[1],
			}
			return res
		}
		if pTry > p {
			maxIndex = try
		} else {
			minIndex = try
		}
		if maxIndex - minIndex <= 1 {
			// The peano could not be found
			// so return the prev and next links of the try
			links := pi.Links[pTry]
			res := binaryResults{
				found: false,
				prevIndex: links[0],
				nextIndex: links[1],
			}
			return res
		}
	}
}

func highBits(p Peano) uint16 {
	// return uint16(uint32(p) / uint32(max16bit))
	return uint16(uint32(p) >> 16)
}
