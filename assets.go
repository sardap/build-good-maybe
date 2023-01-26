package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ettle/strcase"
	_ "github.com/oov/psd"
	"golang.org/x/image/bmp"
)

func init() {
	rand.Seed(time.Now().UnixMicro())
}

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

func LoadPsd(ctx context.Context, inputFile string) (image.Image, error) {
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

	return img, nil
}

func LoadPng(ctx context.Context, inputFile string) (image.Image, error) {
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

	return img, nil
}

func LoadAseprite(ctx context.Context, inFile string) (image.Image, error) {
	fileName := fmt.Sprintf("bgm_aseprite_%d.png", rand.Int())
	defer os.Remove(fileName)

	cmd := exec.CommandContext(ctx, asepritePath, inFile, "--batch", "--sheet", fileName)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)

	f, _ := os.Open(fileName)
	img, _, err := image.Decode(f)
	defer f.Close()

	return img, err
}

func loadImage(ctx context.Context, inFile string) (image.Image, error) {
	switch filepath.Ext(inFile) {
	case ".psd":
		return LoadPsd(ctx, inFile)
	case ".aseprite":
		return LoadAseprite(ctx, inFile)
	case ".png":
		return LoadPng(ctx, inFile)
	default:
		return nil, fmt.Errorf("unknown type given %s", filepath.Ext(inFile))
	}
}

func generatePng(ctx context.Context, inFile, OutFile string) error {
	img, err := loadImage(ctx, inFile)
	if err != nil {
		return err
	}

	os.Remove(strings.TrimSuffix(OutFile, ".png") + ".c")
	os.Remove(strings.TrimSuffix(OutFile, ".png") + ".h")

	f, _ := os.Create(OutFile)
	defer f.Close()
	return png.Encode(f, img)
}

func generateBmp(ctx context.Context, inFile, OutFile string) error {
	img, err := loadImage(ctx, inFile)
	if err != nil {
		return err
	}

	palettedImage := image.NewPaletted(img.Bounds(), palette.Plan9)
	draw.Draw(palettedImage, palettedImage.Rect, img, img.Bounds().Min, draw.Over)

	f, _ := os.Create(OutFile)
	defer f.Close()
	return bmp.Encode(f, palettedImage)
}

type Anime struct {
	For    string
	Size   string
	Frames []string
}

type GraphicsOutput struct {
	Name           string
	Mode           string
	Options        []string
	Files          []string
	Animes         []Anime
	outputtedFiles []string
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

func createAssetName(file, generatePath, ext string) (outFile string) {
	for _, split := range strings.Split(strings.TrimSuffix(file, filepath.Ext(file)), "/") {
		outFile += strcase.ToPascal(split)
	}
	outFile += ext
	outFile = filepath.Join(generatePath, outFile)
	return
}

type GenerateFun func(context.Context, string, string) error

func generateInBetweenFiles(ctx context.Context, genFun GenerateFun, files []string, assetsPath, generatePath, ext string) ([]string, error) {
	errCh := make(chan error, len(files))
	var generatedFiles []string
	for _, file := range files {
		outFile := createAssetName(file, generatePath, ext)
		go func(inFile, outFile string) {
			errCh <- genFun(ctx, inFile, outFile)
		}(filepath.Join(assetsPath, file), outFile)
		generatedFiles = append(generatedFiles, outFile)
	}
	for i := 0; i < len(files); i++ {
		err := <-errCh
		if err != nil {
			return nil, err
		}
	}

	return generatedFiles, nil
}

var (
	gritLock *sync.Mutex = &sync.Mutex{}
)

func (g *GraphicsOutput) generateGrit(ctx context.Context, assetsPath, generatePath string) error {
	generatedFiles, err := generateInBetweenFiles(ctx, generatePng, g.Files, assetsPath, generatePath, ".png")
	if err != nil {
		return err
	}
	defer func(generatedFiles []string) {
		for _, file := range generatedFiles {
			os.Remove(file)
		}
	}(generatedFiles)

	for _, option := range g.Options {
		if len(option) > 2 && option[0:2] == "-O" {
			fmt.Printf("%s\n", filepath.Join(generatePath, option[2:]+".h"))
			os.Remove(filepath.Join(generatePath, option[2:]+".h"))
			os.Remove(filepath.Join(generatePath, option[2:]+".s"))
			os.Remove(filepath.Join(generatePath, option[2:]+".c"))
		}
	}

	args := append(generatedFiles, g.Options...)
	cmd := exec.Command(gritCmdPath, args...)
	cmd.Dir = generatePath
	gritLock.Lock()
	defer gritLock.Unlock()
	err = cmd.Run()
	return err
}

func bmp2gbaCreateHeader(name string, cFile string) []byte {
	re := regexp.MustCompile(`const unsigned ((short)|(char)) (.*)\[(?P<length>\d+)\]`)

	builder := strings.Builder{}

	builder.WriteString(
		"// Auto generated by build good maybe on ",
	)
	builder.WriteString(time.Now().String())
	builder.WriteRune('\n')
	builder.WriteRune('\n')

	builder.WriteString("#ifndef ")
	builder.WriteString(strings.ToUpper(name))
	builder.WriteString("_H\n")

	builder.WriteString("#define ")
	builder.WriteString(strings.ToUpper(name))
	builder.WriteString("_H\n\n")

	for _, match := range re.FindAllStringSubmatch(cFile, -1) {
		name := match[4]
		len := match[5]

		builder.WriteString("#define ")
		builder.WriteString(name)
		builder.WriteString("Len ")
		builder.WriteString(len)
		builder.WriteString("\n")

		builder.WriteString("extern ")
		builder.WriteString(match[0])
		builder.WriteString(";\n\n")

	}

	builder.WriteString("#endif")
	builder.WriteString(" // ")
	builder.WriteString(strings.ToUpper(name))
	builder.WriteString("_H\n")

	return []byte(builder.String())
}

func (g *GraphicsOutput) generateBmp2Gba(ctx context.Context, assetsPath, generatePath string) error {
	generatedFiles, err := generateInBetweenFiles(ctx, generateBmp, g.Files, assetsPath, generatePath, ".bmp")
	if err != nil {
		return err
	}
	defer func(generatedFiles []string) {
		for _, file := range generatedFiles {
			os.Remove(file)
		}
	}(generatedFiles)

	buffer := &bytes.Buffer{}

	args := append(g.Options, generatedFiles...)
	cmd := exec.Command(bmp2gbaPath, args...)
	cmd.Dir = generatePath
	cmd.Stdout = buffer
	err = cmd.Run()

	os.WriteFile(filepath.Join(generatePath, g.Name+".h"), bmp2gbaCreateHeader(g.Name, buffer.String()), 0777)
	os.WriteFile(filepath.Join(generatePath, g.Name+".c"), buffer.Bytes(), 0777)
	return err
}

func (g *GraphicsOutput) generateAnimes(ctx context.Context, assetsPath, generatePath string) error {
	fmt.Printf("Generating animes for %s\n", g.Name)
	defer fmt.Printf("Done generating animes for %s\n", g.Name)

	if len(g.Animes) == 0 {
		return nil
	}

	builder := &strings.Builder{}
	name := fmt.Sprintf("%s Frames", g.Name)

	builder.WriteString(
		"// Auto generated by build good maybe on ",
	)
	builder.WriteString(time.Now().String())
	builder.WriteRune('\n')
	builder.WriteRune('\n')

	builder.WriteString("#ifndef ")
	builder.WriteString(strcase.ToSNAKE(name))
	builder.WriteString("_H\n")

	builder.WriteString("#define ")
	builder.WriteString(strcase.ToSNAKE(name))
	builder.WriteString("_H\n")
	builder.WriteString("#endif\n\n")

	for _, anime := range g.Animes {
		builder.WriteString(fmt.Sprintf("#include \"%s\"\n", anime.For))
	}

	builder.WriteString("\n\n")

	for _, anime := range g.Animes {
		fmt.Printf("Making anime for %s\n", anime.For)
		varName := fmt.Sprintf(
			"%sTiles",
			strings.Split(anime.For, ".")[0],
		)
		baseName := fmt.Sprintf(
			"ANIME_%s",
			strcase.ToCamel(strings.Split(anime.For, ".")[0]),
		)
		stepSize := func() int {
			splits := strings.Split(anime.Size, "x")
			lStr := splits[0]
			l, _ := strconv.ParseInt(lStr, 10, 10)
			rStr := splits[1]
			r, _ := strconv.ParseInt(rStr, 10, 10)
			return int(l / 2 * r / 2)
		}()

		for i, frame := range anime.Frames {
			builder.WriteString(
				fmt.Sprintf(
					"const unsigned char* %s = %s + %d;\n\n",
					strcase.ToPascal(baseName+" "+frame), varName, stepSize*i,
				),
			)
		}
	}

	os.WriteFile(filepath.Join(generatePath, strcase.ToPascal(name)+".h"), []byte(builder.String()), 0777)

	return nil
}

func (g *GraphicsOutput) Generate(ctx context.Context, assetsPath, generatePath string) error {
	fmt.Printf("Generating %s\n", g.Name)
	defer fmt.Printf("Done generating %s\n", g.Name)

	type genSig func(context.Context, string, string) error
	var genFunc genSig = nil
	switch g.Mode {
	case "grit":
		genFunc = g.generateGrit
	case "bmp2gba":
		genFunc = g.generateBmp2Gba
	default:
		return fmt.Errorf("unknown mode given %s", g.Mode)
	}
	if err := genFunc(ctx, assetsPath, generatePath); err != nil {
		return nil
	}

	g.generateAnimes(ctx, assetsPath, generatePath)

	return nil
}
