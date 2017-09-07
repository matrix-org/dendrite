// +build go1.6

package main

import "os"

var useVendor = os.Getenv("GO15VENDOREXPERIMENT") == "0" || os.Getenv("GO15VENDOREXPERIMENT") == ""
