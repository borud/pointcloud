// Command xyz-convert converts XYZ point cloud files to PLY format.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/borud/pointcloud/pkg/pointcloud"
)

func main() {
	normalize := flag.Bool("normalize", false, "center and scale points to [-1, 1]")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: xyz-convert [flags] <input.xyz> <output.ply>\n\n")
		fmt.Fprintf(os.Stderr, "Converts XYZ point cloud files to PLY format.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	input := flag.Arg(0)
	output := flag.Arg(1)

	log.Printf("reading %s...", input)
	pc, err := pointcloud.ReadFile(input)
	if err != nil {
		log.Fatalf("error reading %s: %v", input, err)
	}
	log.Printf("read %d points", len(pc.Points))

	if *normalize {
		log.Printf("normalizing...")
		pc.Normalize()
	}

	log.Printf("writing %s...", output)
	f, err := os.Create(output)
	if err != nil {
		log.Fatalf("error creating %s: %v", output, err)
	}
	defer f.Close()

	if err := pointcloud.WritePLY(f, pc); err != nil {
		log.Fatalf("error writing PLY: %v", err)
	}

	log.Printf("done: %d points written to %s", len(pc.Points), output)
}
