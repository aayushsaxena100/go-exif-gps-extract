package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/dsoprea/go-exif/v3"
	exifCommon "github.com/dsoprea/go-exif/v3/common"
	"html/template"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	GPSLongitudeRef = "GPSLongitudeRef"
	GPSLatitudeRef  = "GPSLatitudeRef"
	GPSLongitude    = "GPSLongitude"
	GPSLatitude     = "GPSLatitude"
	tplStr          = `
<!doctype html>
<html>
	<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
	</head>
	<style>th,td { padding: 10px; font-size:25px; font-face:"Courier New"}</style>
	<table border='1' style='border-collapse:collapse'>
		<thead>
			<tr>
				<th>File Path</th>
				<th>Latitude</th>
				<th>Longitude</th>
			</tr>
		</thead>
		<tbody>
			{{range $data, $rows := . }}
				<tr style='padding:5px'>
					<td>{{ $rows.FilePath }}</td>
					<td>{{ $rows.Latitude }}</td>
					<td>{{ $rows.Longitude }}</td>
				</tr>
			{{ end }}
		</tbody>
	</table>
</html>
`
)

var allowedExtensions = []string{".jpeg", ".jpg", ".png", ".gif"}

type exifData struct {
	FilePath, Latitude, Longitude string
}

func main() {
	// Initialise command line flags
	htmlFlag := flag.Bool("html", false, "denotes whether to generate html file")
	csvFlag := flag.Bool("csv", false, "denotes whether to generate csv file")
	rootPath := flag.String("path", "", "root directory path for images")
	flag.Parse()

	var directoryPath = "images" // Default image root directory
	if *rootPath != "" {
		directoryPath = *rootPath // overwrite if path passed as command line arg
	}

	exifDataArr := make([]*exifData, 0) // To store exif data of images - filePath, lat, lng

	// Look for all files in a directory and its sub-directories
	if err := filepath.WalkDir(directoryPath, func(path string, fileInfo fs.DirEntry, err error) error {
		if err != nil {
			log.Println(fmt.Errorf("error while walking directory: %s, with error: %s", path, err))
			return err
		}

		if hasValidExtension(fileInfo.Name()) {
			exifData, extractErr := extractExifDataFromImage(path)
			if extractErr != nil {
				log.Println(extractErr)
			} else {
				exifDataArr = append(exifDataArr, exifData) // add exif data of images
			}
		}
		return err
	}); err != nil {
		log.Println(err)
		return
	}

	if *htmlFlag && !*csvFlag {
		writeToHTML(exifDataArr)
	} else if !*htmlFlag && *csvFlag {
		writeToCSV(exifDataArr)
	} else {
		writeToCSV(exifDataArr)
		writeToHTML(exifDataArr)
	}
}

func writeToCSV(csvDataArr []*exifData) {
	csvFile, err := createFile("exif-data.csv")
	if err != nil {
		log.Println(fmt.Sprintf("Error creating CSV file. Error: %s", err))
		return
	}
	defer closeFile(csvFile)
	csvWriter := csv.NewWriter(csvFile)
	_ = csvWriter.Write([]string{"File Path", "Latitude", "Longitude"}) // Columns to be added to CSV
	for _, csvData := range csvDataArr {
		_ = csvWriter.Write([]string{csvData.FilePath, csvData.Latitude, csvData.Longitude})
	}
	csvWriter.Flush()
}

func writeToHTML(data []*exifData) {
	htmlFile, err := createFile("exif-data.html")
	if err != nil {
		log.Println(fmt.Sprintf("Error creating HTML file. Error: %s", err))
		return
	}
	defer closeFile(htmlFile)

	tpl, err := template.New("table").Parse(tplStr)
	if err != nil {
		panic(err)
	}

	err = tpl.Execute(htmlFile, data)
	if err != nil {
		panic(err)
	}
}

func extractExifDataFromImage(imageFilePath string) (*exifData, error) {
	var (
		latitudeDirection  string
		longitudeDirection string
		latitudeValue      string
		longitudeValue     string
		finalLat           string
		finalLong          string
	)

	data, err := ioutil.ReadFile(imageFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading from file: %s, with error: %s", imageFilePath, err)
	}

	exifInfo, err := exif.SearchAndExtractExif(data)
	if err != nil {
		if err == exif.ErrNoExif {
			return nil, fmt.Errorf("no EXIF data found in the file: %s, with error: %s", imageFilePath, err)
		}
		return nil, fmt.Errorf("error reading exif data from file: %s, with error: %s", imageFilePath, err)
	}

	exifTags, _, err := exif.GetFlatExifDataUniversalSearch(exifInfo, nil, true)
	if err != nil {
		return nil, fmt.Errorf("error fetching flat exif data from rawData: %s, with error: %s", imageFilePath, err)
	}

	for _, exifTag := range exifTags {
		switch exifTag.TagName {
		case GPSLatitudeRef:
			if direction, ok := exifTag.Value.(string); ok {
				latitudeDirection = direction
			}
		case GPSLatitude:
			if gpsPosArr, ok := exifTag.Value.([]exifCommon.Rational); ok {
				latitudeValue = parseGPSPosition(gpsPosArr)
			}
		case GPSLongitudeRef:
			if direction, ok := exifTag.Value.(string); ok {
				longitudeDirection = direction
			}
		case GPSLongitude:
			if gpsPosArr, ok := exifTag.Value.([]exifCommon.Rational); ok {
				longitudeValue = parseGPSPosition(gpsPosArr)
			}
		}
	}

	if latitudeValue+latitudeDirection == "" {
		finalLat = "Not available"
	} else {
		finalLat = latitudeValue + latitudeDirection
	}
	if longitudeValue+longitudeDirection == "" {
		finalLong = "Not available"
	} else {
		finalLong = longitudeValue + longitudeDirection
	}

	csvData := &exifData{
		FilePath:  imageFilePath,
		Latitude:  finalLat,
		Longitude: finalLong,
	}
	return csvData, nil
}

func parseGPSPosition(gpsPosArr []exifCommon.Rational) (position string) {
	if gpsPosArr[0].Denominator != 0 {
		position = strconv.Itoa(int(gpsPosArr[0].Numerator / gpsPosArr[0].Denominator))
	} else {
		position = "0"
	}
	position += "°"
	if gpsPosArr[1].Denominator != 0 {
		position += strconv.Itoa(int(gpsPosArr[1].Numerator / gpsPosArr[1].Denominator))
	} else {
		position += "0"
	}
	position += "'"
	if gpsPosArr[2].Denominator != 0 {
		position += fmt.Sprintf("%.2f''", float32(gpsPosArr[2].Numerator)/float32(gpsPosArr[2].Denominator))
	} else {
		position += "0''"
	}
	return
}

func createFile(fileName string) (*os.File, error) {
	csvFile, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed to create file: %s", err)
	}
	return csvFile, err
}

func closeFile(csvFile *os.File) {
	func(csvFile *os.File) {
		err := csvFile.Close()
		if err != nil {
			log.Println("Error closing csv file")
		}
	}(csvFile)
}

func hasValidExtension(name string) bool {
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
