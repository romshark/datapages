// Shell is missing from the package on Windows.

//go:build unix

package app

const Shell = "/bin/sh"
