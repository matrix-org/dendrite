// +build !go1.6

package main

import "os"

var useVendor = os.Getenv("GO15VENDOREXPERIMENT") == "1"
