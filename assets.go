package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ettle/strcase"
	_ "github.com/oov/psd"
)

type GraphicsOutputMode string

func UpdateGraphicsCache(assetsDir, buildFilePath string) {
	cacheFilePath := filepath.Join(assetsDir, "build_good_maybe_cache.json")
	updatedCache := *gfxCache
	if buildFileStat, err := os.Stat(buildFilePath); err == nil {
		updatedCache.BuildFileMod = buildFileStat.ModTime()
	}

	filepath.Walk(assetsDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			updatedCache.ModTimes[path] = info.ModTime()
			return nil
		})

	jsonPayload, _ := json.Marshal(updatedCache)
	os.WriteFile(cacheFilePath, jsonPayload, 0777)

}

func LoadGraphicsCache(assetsDir, buildFilePath string) {
	cacheFilePath := filepath.Join(assetsDir, "build_good_maybe_cache.json")
	cacheFile, _ := os.ReadFile(cacheFilePath)
	if err := json.Unmarshal(cacheFile, &gfxCache); err != nil {
		gfxCache = &GraphicsCache{
			ModTimes:     map[string]time.Time{},
			BuildFileMod: time.Time{},
		}
	}

	if buildFileStat, err := os.Stat(buildFilePath); err == nil && !buildFileStat.ModTime().Equal(gfxCache.BuildFileMod) {
		gfxCache.ModTimes = make(map[string]time.Time)
	}

	// If any file is missing invalidate the cache
	for file, _ := range gfxCache.ModTimes {
		if _, err := os.Stat(file); err != nil {
			gfxCache.ModTimes = make(map[string]time.Time)
			return
		}
	}
}

type GraphicsCache struct {
	BuildFileMod time.Time
	ModTimes     map[string]time.Time
}

func (g *GraphicsCache) Update(assetPath string, gfx GraphicsOutput) {
	for _, f := range gfx.Files {
		stat, _ := os.Stat(filepath.Join(assetPath, f))
		g.ModTimes[f] = stat.ModTime()
	}
}

func PsdToPng(ctx context.Context, inputFile, outFile string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		panic(err)
	}

	img = GbaConvertImg(img)

	pngFile, _ := os.Create(outFile)
	defer pngFile.Close()
	png.Encode(pngFile, img)

	return nil
}

func PngToPng(ctx context.Context, inputFile, outFile string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		panic(err)
	}

	img = GbaConvertImg(img)

	pngFile, _ := os.Create(outFile)
	defer pngFile.Close()
	png.Encode(pngFile, img)

	return nil
}

func asepriteToPng(ctx context.Context, inFile, outFile string) error {
	cmd := exec.CommandContext(ctx, asepritePath, inFile, "--batch", "--sheet", outFile)
	if err := cmd.Run(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func generatePng(ctx context.Context, inFile, OutFile string) error {
	switch filepath.Ext(inFile) {
	case ".psd":
		return PsdToPng(ctx, inFile, OutFile)
	case ".aseprite":
		return asepriteToPng(ctx, inFile, OutFile)
	case ".png":
		return PngToPng(ctx, inFile, OutFile)
	}

	return fmt.Errorf("unsupported file given %s", inFile)
}

type GraphicsOutput struct {
	Name    string
	Options []string
	Files   []string
}

func (g *GraphicsOutput) Changed(assetsPath string) bool {
	for _, f := range g.Files {
		fixedFilename := f
		fixedFilename = strings.Replace(fixedFilename, "/", string(filepath.Separator), -1)
		fixedFilename = filepath.Join(assetsPath, fixedFilename)
		if stat, err := os.Stat(fixedFilename); err == nil {
			lastMod, ok := gfxCache.ModTimes[fixedFilename]
			if ok && !lastMod.Equal(stat.ModTime()) || !ok {
				return true
			}
		}
	}

	return false
}

var (
	gritLock *sync.Mutex = &sync.Mutex{}
	tmpLock  *sync.Mutex = &sync.Mutex{}
)

func (g *GraphicsOutput) Generate(ctx context.Context, assetsPath, generatePath string) error {
	tmpLock.Lock()
	defer tmpLock.Unlock()
	fmt.Printf("Generating %s\n", g.Name)
	defer fmt.Printf("Done generating %s\n", g.Name)

	errCh := make(chan error, len(g.Files))
	var generatedFiles []string
	for _, file := range g.Files {
		outFile := ""
		for _, split := range strings.Split(strings.TrimSuffix(file, filepath.Ext(file)), "/") {
			outFile += strcase.ToPascal(split)
		}
		outFile += ".png"
		outFile = filepath.Join(generatePath, outFile)
		go func(inFile, outFile string) {
			errCh <- generatePng(ctx, inFile, outFile)
		}(filepath.Join(assetsPath, file), outFile)
		generatedFiles = append(generatedFiles, outFile)
	}
	for i := 0; i < len(g.Files); i++ {
		err := <-errCh
		if err != nil {
			return err
		}
	}
	defer func(generatedFiles []string) {
		for _, file := range generatedFiles {
			os.Remove(file)
		}
	}(generatedFiles)

	args := append(generatedFiles, g.Options...)
	cmd := exec.Command(gritCmdPath, args...)
	cmd.Dir = generatePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	gritLock.Lock()
	defer gritLock.Unlock()
	err := cmd.Run()
	return err
}
