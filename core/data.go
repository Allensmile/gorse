package core

import (
	"archive/zip"
	"bufio"
	"fmt"
	"github.com/zhenghaoz/gorse/base"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DataSet contains the raw Data table and preprocessed Data structures for recommendation models.
type DataSet struct {
	Table
	GlobalMean       float64              // Global mean of ratings
	DenseUserIds     []int                // Dense user IDs of ratings
	DenseItemIds     []int                // Dense item IDs of ratings
	DenseUserRatings []*base.SparseVector // Ratings of each user
	DenseItemRatings []*base.SparseVector // Ratings of each item
	UserIdSet        *base.SparseIdSet    // User ID set
	ItemIdSet        *base.SparseIdSet    // Item ID set
}

// NewDataSet creates a train set from a raw Data set.
func NewDataSet(table Table) *DataSet {
	set := new(DataSet)
	set.Table = table
	set.GlobalMean = table.Mean()
	set.DenseItemIds = make([]int, 0)
	set.DenseUserIds = make([]int, 0)
	// Create ID set
	set.UserIdSet = base.NewSparseIdSet()
	set.ItemIdSet = base.NewSparseIdSet()
	table.ForEach(func(userId, itemId int, rating float64) {
		set.UserIdSet.Add(userId)
		set.ItemIdSet.Add(itemId)
		set.DenseUserIds = append(set.DenseUserIds, set.UserIdSet.ToDenseId(userId))
		set.DenseItemIds = append(set.DenseItemIds, set.ItemIdSet.ToDenseId(itemId))
	})
	// Create user-based and item-based ratings
	set.DenseUserRatings = base.NewDenseSparseMatrix(set.UserCount())
	set.DenseItemRatings = base.NewDenseSparseMatrix(set.ItemCount())
	table.ForEach(func(userId, itemId int, rating float64) {
		userDenseId := set.UserIdSet.ToDenseId(userId)
		itemDenseId := set.ItemIdSet.ToDenseId(itemId)
		set.DenseUserRatings[userDenseId].Add(itemDenseId, rating)
		set.DenseItemRatings[itemDenseId].Add(userDenseId, rating)
	})
	return set
}

// GetDense get the i-th record by <denseUserId, denseItemId, rating>.
func (dataSet *DataSet) GetDense(i int) (int, int, float64) {
	_, _, rating := dataSet.Get(i)
	return dataSet.DenseUserIds[i], dataSet.DenseItemIds[i], rating
}

// UserCount returns the number of users.
func (dataSet *DataSet) UserCount() int {
	return dataSet.UserIdSet.Len()
}

// ItemCount returns the number of items.
func (dataSet *DataSet) ItemCount() int {
	return dataSet.ItemIdSet.Len()
}

// GetUserRatingsSet gets a user's ratings by the sparse user ID. The returned object
// is a map between item sparse IDs and given ratings.
func (dataSet *DataSet) GetUserRatingsSet(userId int) map[int]float64 {
	denseUserId := dataSet.UserIdSet.ToDenseId(userId)
	set := make(map[int]float64)
	dataSet.DenseUserRatings[denseUserId].ForEach(func(i, index int, value float64) {
		itemId := dataSet.ItemIdSet.ToSparseId(index)
		set[itemId] = value
	})
	return set
}

/* Loader */

// LoadDataFromBuiltIn loads a built-in Data set. Now support:
//   ml-100k   - MovieLens 100K
//   ml-1m     - MovieLens 1M
//   ml-10m    - MovieLens 10M
//   ml-20m    - MovieLens 20M
//   netflix   - Netflix
//   filmtrust - FlimTrust
//   epinions  - Epinions
func LoadDataFromBuiltIn(dataSetName string) *DataSet {
	// Extract Data set information
	dataSet, exist := builtInDataSets[dataSetName]
	if !exist {
		log.Fatal("no such Data set ", dataSetName)
	}
	dataFileName := filepath.Join(dataSetDir, dataSet.path)
	if _, err := os.Stat(dataFileName); os.IsNotExist(err) {
		zipFileName, _ := downloadFromUrl(dataSet.url, downloadDir)
		if _, err := unzip(zipFileName, dataSetDir); err != nil {
			panic(err)
		}
	}
	return dataSet.loader(dataFileName, dataSet.sep, dataSet.header)
}

// LoadDataFromCSV loads Data from a CSV file. The CSV file should be:
//   [optional header]
//   <userId 1> <sep> <itemId 1> <sep> <rating 1> <sep> <extras>
//   <userId 2> <sep> <itemId 2> <sep> <rating 2> <sep> <extras>
//   <userId 3> <sep> <itemId 3> <sep> <rating 3> <sep> <extras>
//   ...
// For example, the `u.Data` from MovieLens 100K is:
//  196\t242\t3\t881250949
//  186\t302\t3\t891717742
//  22\t377\t1\t878887116
func LoadDataFromCSV(fileName string, sep string, hasHeader bool) *DataSet {
	users := make([]int, 0)
	items := make([]int, 0)
	ratings := make([]float64, 0)
	// Open file
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	// Read CSV file
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Ignore header
		if hasHeader {
			hasHeader = false
			continue
		}
		fields := strings.Split(line, sep)
		// Ignore empty line
		if len(fields) < 2 {
			continue
		}
		user, _ := strconv.Atoi(fields[0])
		item, _ := strconv.Atoi(fields[1])
		rating, _ := strconv.ParseFloat(fields[2], 32)
		users = append(users, user)
		items = append(items, item)
		ratings = append(ratings, rating)
	}
	return NewDataSet(NewDataTable(users, items, ratings))
}

// LoadDataFromNetflix loads Data from the Netflix dataset. The file should be:
//   <itemId 1>:
//   <userId 1>, <rating 1>, <date>
//   <userId 2>, <rating 2>, <date>
//   <userId 3>, <rating 3>, <date>
//   ...
func LoadDataFromNetflix(fileName string, _ string, _ bool) *DataSet {
	users := make([]int, 0)
	items := make([]int, 0)
	ratings := make([]float64, 0)
	// Open file
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	// Read file
	scanner := bufio.NewScanner(file)
	itemId := -1
	for scanner.Scan() {
		line := scanner.Text()
		if line[len(line)-1] == ':' {
			// <itemId>:
			if itemId, err = strconv.Atoi(line[0 : len(line)-1]); err != nil {
				log.Fatal(err)
			}
		} else {
			// <userId>, <rating>, <date>
			fields := strings.Split(line, ",")
			userId, _ := strconv.Atoi(fields[0])
			rating, _ := strconv.Atoi(fields[1])
			users = append(users, userId)
			items = append(items, itemId)
			ratings = append(ratings, float64(rating))
		}
	}
	return NewDataSet(NewDataTable(users, items, ratings))
}

/* Utils */

// downloadFromUrl downloads file from URL.
func downloadFromUrl(src string, dst string) (string, error) {
	fmt.Printf("Download dataset from %s\n", src)
	// Extract file name
	tokens := strings.Split(src, "/")
	fileName := filepath.Join(dst, tokens[len(tokens)-1])
	// Create file
	if err := os.MkdirAll(filepath.Dir(fileName), os.ModePerm); err != nil {
		return fileName, err
	}
	output, err := os.Create(fileName)
	if err != nil {
		fmt.Println("Error while creating", fileName, "-", err)
		return fileName, err
	}
	defer output.Close()
	// Download file
	response, err := http.Get(src)
	if err != nil {
		fmt.Println("Error while downloading", src, "-", err)
		return fileName, err
	}
	defer response.Body.Close()
	// Save file
	_, err = io.Copy(output, response.Body)
	if err != nil {
		fmt.Println("Error while downloading", src, "-", err)
		return fileName, err
	}
	return fileName, nil
}

// unzip zip file.
func unzip(src string, dst string) ([]string, error) {
	fmt.Printf("Unzip dataset %s\n", src)
	var fileNames []string
	// Open zip file
	r, err := zip.OpenReader(src)
	if err != nil {
		return fileNames, err
	}
	defer r.Close()
	// Extract files
	for _, f := range r.File {
		// Open file
		rc, err := f.Open()
		if err != nil {
			return fileNames, err
		}
		// Store filename/path for returning and using later on
		filePath := filepath.Join(dst, f.Name)
		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(filePath, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fileNames, fmt.Errorf("%s: illegal file path", filePath)
		}
		// Add filename
		fileNames = append(fileNames, filePath)
		if f.FileInfo().IsDir() {
			// Create folder
			if err = os.MkdirAll(filePath, os.ModePerm); err != nil {
				return fileNames, err
			}
		} else {
			// Create all folders
			if err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return fileNames, err
			}
			// Create file
			outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return fileNames, err
			}
			// Save file
			_, err = io.Copy(outFile, rc)
			// Close the file without defer to close before next iteration of loop
			outFile.Close()
			if err != nil {
				return fileNames, err
			}
		}
		// Close file
		rc.Close()
	}
	return fileNames, nil
}
