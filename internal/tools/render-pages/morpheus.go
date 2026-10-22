package main

import "runtime/debug"

const morpheusModule = "github.com/romshark/morpheus"

// morpheusAssets is the URL of the directory on jsDelivr that holds the
// stylesheet and the script of Morpheus. It names the version of the module
// whose templ components the pages render, which the script must match.
var morpheusAssets = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == morpheusModule {
				return "https://cdn.jsdelivr.net/gh/romshark/morpheus@" +
					d.Version + "/min/"
			}
		}
	}
	panic("render-pages: the build info lists no " + morpheusModule)
}()
