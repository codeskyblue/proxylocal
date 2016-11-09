// Update in CHNAGELOG.md

package main

import (
	"fmt"
	"runtime"
)

const PXVER = "1.1.2"

var VERSION = fmt.Sprintf("Proxylocal version %s, %s\nHomepage: %s",
	PXVER, runtime.Version(), "https://github.com/codeskyblue/proxylocal")
