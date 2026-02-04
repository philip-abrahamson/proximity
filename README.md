# Proximity

A high performance geospatial search engine (filters with a boolean "OR" query) written in Go.

## Introduction

Proximity is a high performance geospatial search engine written in Go / Golang which identifies records
near to a search location, and can perform some simple boolean "OR" logic to filter records.  It is optimised
for speed over accuracy, and is more suitable for certain applications than others.  For instance, it wouldn't
be recommended for a geographic asset identifier, but could work well for a consumer facing search of
e.g. coffee shops.

It is memory based, and frighteningly fast. On an i7, 16G RAM machine, with a million test records, each
test search on the backend takes about 20 microseconds on average (~50,000 per second).  Given that the
API server makes use of multiple CPUs and threads, the total throughput is more likely to be limited by
bandwidth than CPU.

The reason for its high performance, scalability, and sometimes indifferent accuracy is because it
uses a "fractal space filling curve" or "Peano" curve, named after the 19th century Italian
mathematician Giuseppe Peano.  While an accurate, but much slower, proximity search will
query the two dimensions of latitude and longitude for records, a fractal space filling curve
is a one dimensional curve which snakes around two dimensional space, spiralling in and out,
until it fills up the space to some predetermined resolution.  Each 2D latitude and logitude
location is first converted into a one dimensional "Peano code" integer representing the distance
along the curve.  Querying a one dimensional integer index to obtain an approximation of
proximity is much quicker than e.g. querying the differences between 2D locations of a
search and a collection of records.

Our Proximity engine improves on some of the inaccuracy of a single Peano curve by:

(1) Using two curves, offset from each other to help minimise some of the issues
encountered with a single curve when large "jumps" in the curves occur.

(2) Using a traditional 2D proximity approach once a small subset of candidate search
records have been obtained using the Peano curves.

## Installation

$ go build .

This will create a single executable file called "proximity" in the source directory.

## Use

$ ./proximity

## Deployment

Proximity is a Gin application, and can be deployed using Gin's instructions here:

[Gin application deployment instructions] (https://gin-gonic.com/en/docs/deployment/)

## Data Import

On start-up, the executable "proximity" imports data from a CSV file,
which by default should be named "proximity.csv" in the same directory.
See proximity.csv in this code for a small data sample.
Note that IDs are optional, and will become an ascending integer count
if left blank (although they are considered strings). If IDs are included,
they must be unique across the record set.
This CSV data is parsed & read into memory, and will persist for the lifetime of
the process.  If you make updates to the CSV file you will need to
restart the proximity executable for those changes to apply.

## Configuration

Environment variables:

MODE - debug, release, or test
PORT - defaults to 8080
DATAFILE - defaults to "proximity.csv", is the filepath to
           the CSV file to import.
MAX_RESULTS - defaults to 20. Searches will return this number of
              results or fewer
UNITS - defaults to "km", but can also be set to "mi" for miles.

## Tests

Run the tests with:

$ go test -v -count=1 ./...



## Boolean Filtering

Currently you can apply a limited boolean "OR" filter to the search.
There are plans to extend this in future, and make this friendlier
to use.

For the current version of this, you have the ability to set a series
of 64 arbitrary boolean flags in a bitmap, and search for these using
a matching bitmask.

For example, if you are setting up a restaurant search and want to be able to
filter the restaurants by whether they are vegan or not, you could use the
first of the 64 flags to indicate this.

A vegan restaurant will have a Bitmap field which is an unsigned 64bit
integer which we'll represent here in hexadecimal with the first bit
set as: 0x0000000000000001

Then to search for vegan restaurants, the search bitmask
will also be 0x0000000000000001

Note that this is currently limited to OR logic only.

If you wanted to mark restaurants as Indian with another flag,
so e.g. the Bitmap of an Indian Vegan restaurant was

0x0000000000000011

This record would appear if you were searching for either

Vegan restaurants: 0x0000000000000001

or

Indian restaurants: 0x0000000000000010

or

Vegan OR Indian restaurants: 0x0000000000000011


## Copyright & Licensing

Copyright Philip Abrahamson 2025-2026
Copyright High Country Software Ltd 2002-2004

Licensed under the GNU General Public License version 2.0 (GPLv2)

This software borrows some strategies and code which were authored around the turn
of the century by High Country Software Ltd, whose development team consisted of
myself and my brother, Peter Abrahamson.  That older code was originally open-sourced
under a GPL v2 licence, which I've attached to this new code.
