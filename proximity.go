// Copyright Philip Abrahamson 2025-2026
// Copyright High Country Software Ltd 2002-2004
//
// Licensed under the GNU General Public License version 2.0 (GPLv2)

package main

import (
	"localhost/proximity/geodata"
	"github.com/aviddiviner/gin-limit"
	"github.com/gin-gonic/gin"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
)

// TODO - instead rely on, but document, the env variables used by Gin https://gin-gonic.com/en/docs/deployment/
const Debug = true

const DefaultDataFile = "proximity.csv"
const DefaultMaxResults = 20
const LimitMaxResults = 100
const FloatSize = 64
const BitmaskSize = 64
const MaxResultsSize = 64

type Job struct {
	Lat float64
	Lon float64
	Bitmask uint64
	Results chan<- geodata.Results
}

func main() {

	if Debug {
		gin.SetMode(gin.ReleaseMode)
	} else {
		log.Print("proximity is now in debug mode")
	}

	// generate the proximity data & indices from a CSV file
	geo := new(geodata.GeoData)
	err := geo.Import( datafile() )
	if err != nil {
		panic(err)
	}

	// initialise the proximity engine worker pool
	jobs, size := initPool(geo)

	// Gin router with default middleware (logger and recovery)
	router := gin.Default()
	router.SetTrustedProxies(nil)

	router.Use(attachData(geo))

	// limit the maximum number of simultaneous API requests
	// to that of the proximity engine pool size
	router.Use(limit.MaxAllowed(size))

	// Proximity search endpoint
	router.GET("/", func(context *gin.Context) {

		lat, lon, bitmask, err := parseParams(context)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// create a channel to receive the proximity search result
		res := make(chan geodata.Results)

		// post this proximity search as a job for the pool of workers to pick up
		job := Job{ Lat: lat, Lon: lon, Bitmask: bitmask, Results: res }
		postJob(jobs, job)

		// block until we get the results
		results := <-res

		if Debug {
			context.IndentedJSON(http.StatusOK, results)
			log.Print("Results:")
			log.Print(results)
		} else {
			context.JSON(http.StatusOK, results)
		}
	})

	// Start server on port 8080 (default)
	router.Run()
}

func datafile() string {
	file := os.Getenv("DATAFILE")
	if file != "" {
		return file
	}
	return DefaultDataFile
}

func maxResults() uint64 {
	maxStr := os.Getenv("MAX_RESULTS")
	if maxStr != "" {
		maxInt, err := strconv.ParseUint(maxStr, 0, MaxResultsSize)
		if err != nil {
			panic("Failed to parse the input integer environment variable MAX_RESULTS")
		}
		if maxInt > LimitMaxResults {
			panic(fmt.Sprintf("The input integer environment variable MAX_RESULTS must be no more than %d", LimitMaxResults))
		}
		return maxInt
	}
	return DefaultMaxResults
}

func units() string {
	units := os.Getenv("UNITS")
	if units != "mi" {
		units = "km"
	}
	return units
}

// Gin middleware to attach our geodata to
// each Gin handler
func attachData(geo *geodata.GeoData) gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Set("geodata", geo)
	}
}

func parseParams(context *gin.Context) (lat, lon float64, bitmask uint64, err error) {
	for k, v := range map[string]*float64 {"lat": &lat, "lon": &lon} {
		param := context.Query(k)
		*v, err = strconv.ParseFloat(param, FloatSize)
		if err != nil {
			if Debug {
				log.Printf("Error converting %s '%s' to a float - %s\n", k, param, err.Error())
			}
			// Not err.Error() here, because it would reveal system details to the user
			return 0, 0, 0, fmt.Errorf("Error converting %s '%s' to a float", k, param)
		}
	}
	bitmaskStr := context.Query("bitmask")
	bitmask, err = strconv.ParseUint(bitmaskStr, 0, BitmaskSize)
	if err != nil {
		if Debug {
			log.Printf("Error converting bitmask '%s' to a uint - %s\n", bitmaskStr, err.Error())
		}
		// Not err.Error() here, because it would reveal system details to the user
		return 0, 0, 0, fmt.Errorf("Error converting bitmask '%s' to an integer", bitmaskStr)
	}
	return lat, lon, bitmask, nil
}

func initPool(geo *geodata.GeoData) (jobs chan Job, size int) {
	size = poolSize()
	jobs = make(chan Job, size)
	for i := 0; i < size; i++ {
		go worker(geo, jobs, i)
	}
	if Debug {
		log.Printf("Pool of %d proximity workers initialised\n", size)
	}
	return jobs, size
}

func poolSize() int {
	return runtime.NumCPU()
}

func postJob(jobs chan<- Job, job Job) {
	jobs <- job
	return
}

func worker(geo *geodata.GeoData, jobs <-chan Job, i int) {
	// each worker will grab any available job
	for job := range jobs {
		processJob(geo, job)
	}
}

func processJob(geo *geodata.GeoData, job Job) {
	lat := job.Lat
	lon := job.Lon
	bitmask := job.Bitmask
	if Debug {
		log.Printf("Searching: lat = %0.6f, lon = %0.6f, bitmask = %v\n", lat, lon, bitmask)
	}

	// Make the geospatial query
	// TODO - bitmask in future might instead be a boolean logic expression...
	res := geo.Find(lat, lon, bitmask, maxResults(), units())

	// post the results back to the results channel in the job
	job.Results <- res

	return
}
