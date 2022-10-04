package main

import (
	"context"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type BuildFile struct {
	Graphics []GraphicsOutput
}

func (b *BuildFile) Generate(assetsPath, generatePath string) error {
	workQueue := []GraphicsOutput{}
	for _, out := range b.Graphics {
		if out.Changed(assetsPath) {
			workQueue = append(workQueue, out)
		} else {
			fmt.Printf("Skipping %s no changes found\n", out.Name)
		}
	}

	ctx := context.Background()
	errCh := make(chan error, len(workQueue))
	for _, out := range workQueue {
		go func(out GraphicsOutput) {
			errCh <- out.Generate(ctx, assetsPath, generatePath)
		}(out)
	}
	for i := 0; i < len(workQueue); i++ {
		if err := <-errCh; err != nil && err.Error() != "exit status 0xc0000374" {
			return err
		}
	}

	return nil
}

func loadBuildFile(buildFilePath string) BuildFile {
	buildFile, err := os.ReadFile(buildFilePath)
	if err != nil {
		panic(err)
	}

	var config BuildFile
	if err := toml.Unmarshal(buildFile, &config); err != nil {
		panic(err)
	}

	return config
}
