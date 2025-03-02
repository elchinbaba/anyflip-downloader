package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/br3w0r/goitopdf/itopdf"
	"github.com/schollz/progressbar/v3"
)

var title string
var tempDownloadFolder string
var insecure bool
var pageCount int

type flipbook struct {
	URL       *url.URL
	title     string
	pageCount int
	pageURLs  []string
}

func init() {
	flag.StringVar(&tempDownloadFolder, "temp-download-folder", "", "Specifies the name of the temporary download folder")
	flag.StringVar(&title, "title", "", "Specifies the name of the generated PDF document (uses book title if not specified)")
	flag.BoolVar(&insecure, "insecure", false, "Skip certificate validation")
	flag.IntVar(&pageCount, "page-count", 0, "Specifies the number of pages of the generated PDF document (uses auto calculated number if not specified (might return error)")

	flag.Parse()
}

func main() {
	anyflipURL, err := url.Parse(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}
	
	if insecure {
		fmt.Println("You enabled insecure downloads. This disables security checks. Stay safe!")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	
	if len(flag.Args()) > 2 && flag.Args()[1] == "-page-count" {
		pageCount, err = strconv.Atoi(flag.Args()[2])
	}

	fmt.Println("Preparing to download")
	flipbook, err := prepareDownload(anyflipURL)
	if err != nil {
		log.Fatal(err)
	}

	if tempDownloadFolder == "" {
		tempDownloadFolder = flipbook.title
	}
	outputFile := title + ".pdf"

	err = flipbook.downloadImages(tempDownloadFolder)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Converting to pdf")
	err = createPDF(outputFile, tempDownloadFolder)
	if err != nil {
		log.Fatal(err)
	}

	os.RemoveAll(tempDownloadFolder)
}

func prepareDownload(anyflipURL *url.URL) (*flipbook, error) {
	var newFlipbook flipbook

	sanitizeURL(anyflipURL)
	newFlipbook.URL = anyflipURL

	configjs, err := downloadConfigJSFile(anyflipURL)
	if err != nil {
		return nil, err
	}

	if title == "" {
		title, err = getBookTitle(configjs)
		if err != nil {
			title = path.Base(anyflipURL.String())
		}
	}

	newFlipbook.title = govalidator.SafeFileName(title)
	
	if pageCount > 0 {
		newFlipbook.pageCount = pageCount
	} else {
		newFlipbook.pageCount, err = getPageCount(configjs)
	}
	
	pageFileNames := getPageFileNames(configjs)

	downloadURL, _ := url.Parse("https://online.anyflip.com/")
	println(newFlipbook.URL.String())
	if len(pageFileNames) == 0 {
		for i := 1; i <= newFlipbook.pageCount; i++ {
			downloadURL.Path = path.Join(newFlipbook.URL.Path, "files", "mobile", strconv.Itoa(i)+".jpg")
			newFlipbook.pageURLs = append(newFlipbook.pageURLs, downloadURL.String())
		}
	} else {
		for i := 0; i < newFlipbook.pageCount; i++ {
			downloadURL.Path = path.Join(newFlipbook.URL.Path, "files", "large", pageFileNames[i])
			newFlipbook.pageURLs = append(newFlipbook.pageURLs, downloadURL.String())
		}
	}

	return &newFlipbook, err
}

func sanitizeURL(anyflipURL *url.URL) {
	bookURLPathElements := strings.Split(anyflipURL.Path, "/")
	anyflipURL.Path = path.Join("/", bookURLPathElements[1], bookURLPathElements[2])
}

func createPDF(outputFile string, imageDir string) error {
	pdf := itopdf.NewInstance()
	err := pdf.WalkDir(imageDir, nil)
	if err != nil {
		return err
	}
	err = pdf.Save(outputFile)
	if err != nil {
		return err
	}
	return nil
}

func (fb *flipbook) downloadImages(downloadFolder string) error {
	err := os.Mkdir(downloadFolder, os.ModePerm)
	if err != nil {
		return err
	}

	bar := progressbar.NewOptions(fb.pageCount,
		progressbar.OptionFullWidth(),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription("Downloading"),
	)

	for page := 0; page < fb.pageCount; page++ {
		downloadURL := fb.pageURLs[page]
		response, err := http.Get(downloadURL)
		if err != nil {
			return err
		}

		if response.StatusCode != http.StatusOK {
			println("During download from ", downloadURL)
			return errors.New("Received non-200 response: " + response.Status)
		}

		extension := path.Ext(downloadURL)
		filename := fmt.Sprintf("%04d%v", page, extension)
		file, err := os.Create(path.Join(downloadFolder, filename))
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(file, response.Body)
		if err != nil {
			return err
		}

		bar.Add(1)
	}
	fmt.Println()
	return nil
}

func downloadConfigJSFile(bookURL *url.URL) (string, error) {
	configjsURL, err := url.Parse("https://online.anyflip.com")
	if err != nil {
		return "", err
	}
	configjsURL.Path = path.Join(bookURL.Path, "mobile", "javascript", "config.js")
	resp, err := http.Get(configjsURL.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("received non-200 response:" + resp.Status)
	}
	configjs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(configjs), nil
}
