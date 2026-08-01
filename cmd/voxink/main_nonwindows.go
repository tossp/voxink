//go:build !windows

// Command voxink starts the VoxInk desktop application.
package main

import "fmt"

func main() {
	handleSelfCheck()
	fmt.Println("VoxInk stage 1 is Windows-only; this build is a local scaffold")
}
