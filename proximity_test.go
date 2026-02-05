// Copyright Philip Abrahamson 2025-2026
// Copyright High Country Software Ltd 2002-2004
//
// Licensed under the GNU General Public License version 2.0 (GPLv2)

package main

import (
	"testing"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"localhost/proximity/geodata"
	"github.com/stretchr/testify/assert"
)

// Sanity check of an API call
//
// Note: see geodata/test_geodata.go for search functionality tests
//
func TestAPI(t *testing.T) {

	router := setupRouter()

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/?lat=1.23456&lon=-1.23456&bitmask=0", nil)
	router.ServeHTTP(res, req)

	assert := assert.New(t)
	assert.Equal(200, res.Code, "API call returned 200")
	var results geodata.Results
	decoder := json.NewDecoder(res.Body)
	err := decoder.Decode(&results)
	assert.Nil(err, "No JSON parsing error")
	if len(results) < 1 {
		t.Errorf("No results array returned")
	}
	t.Logf("%d results returned\n%v", len(results), results)
}
