package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func runMake(workDir string, makeArgs []string) {
	fmt.Printf("Running make\n")
	cmd := exec.Command("make", makeArgs...)
	cmd.Dir = workDir

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	stdIn, _ := cmd.StdinPipe()
	stdIn.Write([]byte("y"))
	stdIn.Close()

	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	if err != nil {
		fmt.Printf("error running make %v\n", err)
		os.Exit(2)
	}

}

func printHelp() {
	fmt.Printf("invalid command given")
}

func main() {
	const (
		outPathIdx       = 1
		buildPathFileIdx = 2
		AssetsPathIdx    = 3
		MakePathIdx      = 4
		makeArgsIdx      = 5
	)
	if len(os.Args) == outPathIdx && len(os.Args) != makeArgsIdx+1 {
		printHelp()
		return
	}

	cmd := os.Args[1]
	if cmd == "-h" {
		printHelp()
		return
	}

	outPath := os.Args[outPathIdx]

	var makeArgs []string
	if len(os.Args) == makeArgsIdx {
		makeArgs = []string{}
	} else {
		makeArgs = os.Args[makeArgsIdx:len(os.Args)]
	}

	buildFilePath := os.Args[buildPathFileIdx]

	AssetsPath := os.Args[AssetsPathIdx]

	LoadGraphicsCache(AssetsPath, buildFilePath)

	buildFile := loadBuildFile(buildFilePath)

	if err := buildFile.Generate(AssetsPath, outPath); err != nil {
		panic(err)
	}

	makePath := os.Args[MakePathIdx]

	runMake(makePath, makeArgs)

	UpdateGraphicsCache(AssetsPath, buildFilePath)
}
