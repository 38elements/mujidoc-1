package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/japanese-document/mujidoc/internal/css"
	"github.com/japanese-document/mujidoc/internal/utils"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func init() {
	err := godotenv.Load(".env.mujidoc")
	if err != nil {
		log.Fatal(err)
	}
}

func createPageHtmlFileTask(markDownFileName, indexMenu, pageLayout, sourceDir, outputDir, baseUrl string) func() error {
	return func() error {
		content, err := os.ReadFile(markDownFileName)
		if err != nil {
			return errors.WithStack(err)
		}
		_, md, err := utils.GetMetaAndMd(string(content))
		if err != nil {
			return err
		}
		title := utils.CreateTitle(md)
		dir, name := utils.GetDirAndName(markDownFileName)
		url := utils.CreateURL(dir, name, sourceDir, baseUrl)
		headerList, err := utils.CreateHeaderList(md)
		if err != nil {
			return err
		}
		cssPath := fmt.Sprintf("%s/app.css?v=%s", baseUrl, css.Version())
		page, err := utils.CreatePage(pageLayout, md, title, url, cssPath, indexMenu, headerList)
		if err != nil {
			return err
		}
		dirPath := utils.CreateHTMLFileDir(dir, sourceDir, outputDir)
		if !utils.IsDirExists(dirPath) {
			err := os.MkdirAll(dirPath, os.ModePerm)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		htmlFileName := filepath.Join(dirPath, fmt.Sprintf("%s.html", name))
		err = os.WriteFile(htmlFileName, []byte(page), 0644)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
}

func createCopyImageDirTask(sourceDir, outputDir string) func() error {
	return func() error {
		srcImageDir := filepath.Join(sourceDir, utils.IMAGE_DIR)
		if utils.IsDirExists(srcImageDir) {
			return utils.CopyDir(srcImageDir, outputDir)
		}
		return nil
	}
}

func createIndexHtmlFileTask(layout, outputDir, baseURL, header, title, description, cssPath string, indexItems []utils.IndexItem) func() error {
	return func() error {
		indexPageLayout, err := os.ReadFile(layout)
		if err != nil {
			return errors.WithStack(err)
		}
		indexPage, err := utils.CreateIndexPage(
			string(indexPageLayout), baseURL, header, title, description, cssPath, indexItems)
		if err != nil {
			return err
		}
		htmlFileName := filepath.Join(outputDir, "index.html")
		return os.WriteFile(htmlFileName, []byte(indexPage), 0644)
	}
}

func main() {
	sourceDir := os.Getenv("SOURCE_DIR")
	outputDir := os.Getenv("OUTPUT_DIR")
	baseURL := strings.Trim(os.Getenv("BASE_URL"), "/")
	err := os.RemoveAll(outputDir)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	err = os.Mkdir(outputDir, os.ModePerm)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	markDownFileNames, err := utils.GetMarkDownFileNames(sourceDir)
	if err != nil {
		log.Fatalf("%+v", err)
	}

	pages := []*utils.Page{}
	if os.Getenv("SINGLE_PAGE") != "true" {
		pages, err = utils.CreatePages(markDownFileNames, sourceDir, baseURL)
		if err != nil {
			log.Fatalf("%+v", err)
		}
	}
	indexItems, err := utils.CreateIndexItems(pages)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	indexMenu := utils.CreateIndexMenu(indexItems)
	_pageLayout, err := os.ReadFile(os.Getenv("PAGE_LAYOUT"))
	if err != nil {
		log.Fatalf("%+v", errors.WithStack(err))
	}
	pageLayout := string(_pageLayout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, _ := errgroup.WithContext(ctx)

	// markdownからhtmlを生成する
	for _, markDownFileName := range markDownFileNames {
		task := createPageHtmlFileTask(markDownFileName, indexMenu, pageLayout, sourceDir, outputDir, baseURL)
		eg.Go(task)
	}

	// 画像をコピーする
	task := createCopyImageDirTask(sourceDir, outputDir)
	eg.Go(task)

	// index.htmlを作成する
	if os.Getenv("SINGLE_PAGE") != "true" {
		task = createIndexHtmlFileTask(os.Getenv("INDEX_PAGE_LAYOUT"), outputDir, baseURL, os.Getenv("INDEX_PAGE_HEADER"),
			os.Getenv("INDEX_PAGE_TITLE"), os.Getenv("INDEX_PAGE_DESCRIPTION"), os.Getenv("CSS_PATH"), indexItems)
		eg.Go(task)
	}

	// CSSファイルを作成する
	task = css.CreateWriteTask(outputDir, utils.CSS_FILE_NAME)
	eg.Go(task)

	if os.Getenv("RSS") == "true" {
		task := utils.CreateRssFileTask(pages, os.Getenv("TIME_ZONE"), outputDir, baseURL, os.Getenv("INDEX_PAGE_TITLE"),
			os.Getenv("INDEX_PAGE_DESCRIPTION"))
		eg.Go(task)
	}

	if err := eg.Wait(); err != nil {
		log.Fatalf("%+v", err)
	}
}
