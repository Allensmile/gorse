package core

import (
	"crypto/md5"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zhenghaoz/gorse/base"
	"io"
	"log"
	"os"
	"testing"
)

func md5Sum(fileName string) string {
	// Open file
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	// Generate check sum
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestDownloadFromUrl(t *testing.T) {
	// Download
	fileName, err := downloadFromUrl("https://cdn.sine-x.com/datasets/movielens/ml-100k.zip", downloadDir)
	if err != nil {
		t.Fatal(err)
	}
	// Checksum
	if md5 := md5Sum(fileName); md5 != "0e33842e24a9c977be4e0107933c0723" {
		t.Logf("MD5 sum doesn't match (%s != 0e33842e24a9c977be4e0107933c0723)", md5)
	}
}

func TestUnzip(t *testing.T) {
	// Download
	zipName, err := downloadFromUrl("https://cdn.sine-x.com/datasets/movielens/ml-100k.zip", downloadDir)
	if err != nil {
		t.Fatal("download file failed ", err)
	}
	// Extract files
	fileNames, err := unzip(zipName, dataSetDir)
	// Check
	if err != nil {
		t.Fatal("unzip file failed ", err)
	}
	if len(fileNames) != 24 {
		t.Fatal("Number of file doesn't match")
	}
}

func TestLoadDataFromBuiltIn(t *testing.T) {
	data := LoadDataFromBuiltIn("ml-100k")
	assert.Equal(t, 100000, data.Len())
}

func TestLoadDataFromCSV_Explicit(t *testing.T) {
	data := LoadDataFromCSV("../example/data/implicit.csv", ",", true)
	assert.Equal(t, 5, data.Len())
	for i := 0; i < data.Len(); i++ {
		userId, itemId, value := data.Get(i)
		denseUserId, denseItemId, _ := data.GetDense(i)
		assert.Equal(t, i, userId)
		assert.Equal(t, 2*i, itemId)
		assert.Equal(t, 3*i, int(value))
		assert.Equal(t, i, denseUserId)
		assert.Equal(t, i, denseItemId)
	}
}

func TestLoadDataFromNetflixStyle(t *testing.T) {
	data := LoadDataFromNetflix("../example/data/netflix.txt", ",", true)
	assert.Equal(t, 5, data.Len())
	for i := 0; i < data.Len(); i++ {
		userId, itemId, value := data.Get(i)
		denseUserId, denseItemId, _ := data.GetDense(i)
		assert.Equal(t, 2*i, userId)
		assert.Equal(t, i, itemId)
		assert.Equal(t, 3*i, int(value))
		assert.Equal(t, i, denseUserId)
		assert.Equal(t, i, denseItemId)
	}
}

func TestDataSet_GetUserRatingsSet(t *testing.T) {
	data := DataSet{
		ItemIdSet: &base.SparseIdSet{
			DenseIds:  map[int]int{0: 0, 2: 1, 4: 2, 6: 3},
			SparseIds: []int{0, 2, 4, 6},
		},
		UserIdSet: &base.SparseIdSet{
			DenseIds:  map[int]int{2: 0},
			SparseIds: []int{2},
		},
		DenseUserRatings: []*base.SparseVector{
			{
				Indices: []int{1, 2},
				Values:  []float64{10.0, 20.0},
			},
		},
	}
	set := data.GetUserRatingsSet(2)
	assert.Equal(t, map[int]float64{2: 10, 4: 20}, set)
}
