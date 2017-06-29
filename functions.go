package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

var streamIDtoNumChunks map[uint32]uint32

func init() {
	streamIDtoNumChunks = make(map[uint32]uint32) // cache for numChunks by streamID
}

func loadManifest(manifest string) *Header {
	b, err := ioutil.ReadFile(manifest)
	if err != nil {
		log.Println(err)
	} else {
		hdr := new(Header)
		err = json.Unmarshal(b, hdr)
		if err != nil {
			log.Println(err)
		} else {
			return hdr
		}
	}
	return nil
}

func chunksForStream(tmpDir string, streamID uint32) uint32 {
	numChunks, ok := streamIDtoNumChunks[streamID]
	if ok {
		return numChunks
	}
	// cache miss, check for manifest file
	manifest := filepath.Join(tmpDir, strconv.FormatUint(uint64(streamID), 16), "manifest.json")
	hdr := loadManifest(manifest)
	if hdr != nil {
		streamIDtoNumChunks[streamID] = hdr.NumChunks
		return hdr.NumChunks
	}
	return 0
}

func fileCount(path string) int {
	i := 0
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return 0
	}
	for _, file := range files {
		if !file.IsDir() {
			i++
		}
	}
	return i
}

func mkStreamDir(tmpDir string, streamID uint32) (string, error) {
	// Create Directory for File, if not exist
	tmp := filepath.Join(tmpDir, strconv.FormatUint(uint64(streamID), 16))
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		return tmp, os.Mkdir(tmp, 0700)
	}
	return tmp, nil
}

func isStreamComplete(tmpDir string, streamID, numChunks uint32) bool {
	tmp := filepath.Join(tmpDir, strconv.FormatUint(uint64(streamID), 16))
	if fileCount(tmp)-1 == int(numChunks) { // -1 for the manifest.json file, which is not a chunk
		return true
	}
	return false
}

func completeFile(tmpDir string, streamID uint32, rxDir string) {
	tmp := filepath.Join(tmpDir, strconv.FormatUint(uint64(streamID), 16))
	manifest := filepath.Join(tmp, "manifest.json")
	hdr := loadManifest(manifest)
	if hdr != nil && len(hdr.Filename) > 0 && len(hdr.Filename) < 256 {
		// Open the output file
		outfile, err := os.Create(filepath.Join(rxDir, hdr.Filename))
		defer outfile.Close()
		if err != nil {
			log.Println(err)
			return
		}
		// Copy out all the chunks
		var i uint64
		for i = 0; i < uint64(hdr.NumChunks); i++ {
			chunk := filepath.Join(tmp, strconv.FormatUint(i, 16))
			b, err := ioutil.ReadFile(chunk)
			if err != nil {
				log.Println(err) // this will get hit if a file is missing
				return
			}
			outfile.Write(b)
		}
	}
}
