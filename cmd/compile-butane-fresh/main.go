package main

import (
	"fmt"
	"github.com/projectbluefin/knuckle/internal/ignition"
	"os"
)

func main() {
	data, err := os.ReadFile("/tmp/karnataka-fresh.butane")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	ign, err := ignition.CompileToIgnition(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(ign)
}
