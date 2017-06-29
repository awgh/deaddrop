package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/awgh/bencrypt/bc"
	"github.com/awgh/bencrypt/ecc"
	"github.com/awgh/ratnet/nodes/qldb"
	"github.com/awgh/ratnet/policy"
	"github.com/awgh/ratnet/transports/udp"
)

const (
	hdrMagic      uint32 = 0xF113
	chunkMagic    uint32 = 0xF114
	chunksize     uint32 = 49007
	hdrsize       uint32 = 279
	chunkHdrSize  uint32 = 12
	chunkDataSize uint32 = chunksize - chunkHdrSize
)

// WARNING DANGER TODO FAKE CODE!!!
// PROGRAM IS NOT SAFE TO USE UNTIL THESE KEYS ARE REPLACED!!!
var pubprivkeyb64Ecc = "Tcksa18txiwMEocq7NXdeMwz6PPBD+nxCjb/WCtxq1+dln3M3IaOmg+YfTIbBpk+jIbZZZiT+4CoeFzaJGEWmg=="
var pubkeyb64Ecc = "Tcksa18txiwMEocq7NXdeMwz6PPBD+nxCjb/WCtxq18="

// Header manifest for a file transfer
type Header struct {
	StreamID  uint32
	Filesize  uint64
	Chunksize uint32
	NumChunks uint32
	Filename  string
}

func main() {
	var dbFile, rxDir, txDir, tmpDir string
	var publicPort int

	flag.StringVar(&dbFile, "dbfile", "ratnet.ql", "QL Database File")
	flag.IntVar(&publicPort, "p", 20001, "UDP Public Port (*)")
	flag.StringVar(&rxDir, "rxdir", "inbox", "Download Directory")
	flag.StringVar(&txDir, "txdir", "outbox", "Upload Directory")
	flag.StringVar(&tmpDir, "tempdir", "/tmp", "Temp File Directory")
	//flag.StringVar(&sentDir, "sentdir", "sent", "Completed Uploads Directory")
	flag.Parse()

	listenPublic := fmt.Sprintf(":%d", publicPort)

	// QLDB Node Mode
	node := qldb.New(new(ecc.KeyPair), new(ecc.KeyPair))
	node.BootstrapDB(dbFile)

	// TODO: hardcoded test key because this is TEST PROGRAM until this is fixed.
	if err := node.AddChannel("fixme", pubprivkeyb64Ecc); err != nil {
		log.Fatal(err.Error())
	}

	transportPublic := udp.New(node)

	node.SetPolicy(
		policy.NewP2P(transportPublic, listenPublic, node, false))
	log.Println("Public Server starting: ", listenPublic)

	node.FlushOutbox(0)
	node.Start()

	files, _ := ioutil.ReadDir(txDir)
	for _, f := range files {
		filename := filepath.Join(txDir, f.Name())
		log.Println("Sending ", filename, " with size ", f.Size())

		numChunks64 := f.Size() / int64(chunkDataSize)
		if f.Size()%int64(chunkDataSize) != 0 {
			numChunks64++
		}
		if numChunks64 > math.MaxUint32 {
			log.Println("File is too big, cancelling send")
			continue
		}
		numChunks := uint32(numChunks64)
		log.Println("Sending Header with ", numChunks, " chunks")

		rnd, _ := bc.GenerateRandomBytes(4)
		streamID := binary.LittleEndian.Uint32(rnd)

		hdr := make([]byte, hdrsize)
		binary.LittleEndian.PutUint32(hdr, hdrMagic)
		binary.LittleEndian.PutUint32(hdr[4:], streamID)
		binary.LittleEndian.PutUint64(hdr[8:], uint64(f.Size()))
		binary.LittleEndian.PutUint32(hdr[16:], chunksize)
		binary.LittleEndian.PutUint32(hdr[20:], numChunks)
		copy(hdr[24:], f.Name())
		if err := node.SendChannel("fixme", hdr); err != nil {
			log.Println(err.Error())
			continue
		}

		inputFile, err := os.Open(filename)
		defer inputFile.Close()

		if err != nil {
			log.Println(err.Error())
			continue
		}
		var i uint32
		for i = 0; i < numChunks; i++ {
			chunk := make([]byte, chunksize)
			binary.LittleEndian.PutUint32(chunk, chunkMagic)
			binary.LittleEndian.PutUint32(chunk[4:], streamID)
			binary.LittleEndian.PutUint32(chunk[8:], i)

			n, err := inputFile.ReadAt(chunk[12:], int64(chunkDataSize)*int64(i))
			if (i != numChunks-1 && n != int(chunkDataSize)) || (err != nil && err != io.EOF) {
				log.Println("Chunk could not be read, n=", n, ", i=", i, " ", err.Error())
				break
			}
			log.Println("read ", n, " bytes.  sent ", len(chunk))
			if n > 0 {
				err = node.SendChannel("fixme", chunk[:int(chunkHdrSize)+n])
				if err != nil && err != io.EOF {
					log.Println(err)
					break
				}
			}
		}
		if err != nil {
			log.Println(err.Error())
		} else {
			log.Println("Sending complete, deleting " + filename)
			if err := os.Remove(filename); err != nil {
				log.Println(err)
			}
		}
	}
	for {
		// read a message from the output channel
		msg := <-node.Out()
		log.Println("Receiving stuff!")
		buf := msg.Content.Bytes()
		// read and validate the header
		magic := binary.LittleEndian.Uint32(buf)
		switch magic {

		case hdrMagic:
			// Read Header Fields
			var hdr Header
			hdr.StreamID = binary.LittleEndian.Uint32(buf[4:])
			hdr.Filesize = binary.LittleEndian.Uint64(buf[8:])
			hdr.Chunksize = binary.LittleEndian.Uint32(buf[16:])
			hdr.NumChunks = binary.LittleEndian.Uint32(buf[20:])
			filename := make([]byte, 255)
			copy(filename, buf[24:])
			filename = bytes.TrimRight(filename, string([]byte{0}))
			hdr.Filename = string(filename)

			if tmp, err := mkStreamDir(tmpDir, hdr.StreamID); err != nil {
				log.Println(err)
			} else {
				// Write Manifest File
				b, err := json.Marshal(hdr)
				if err != nil {
					log.Println(err)
				}
				err = ioutil.WriteFile(filepath.Join(tmp, "manifest.json"), b, 0644)
				if err != nil {
					log.Println(err)
				}
				streamIDtoNumChunks[hdr.StreamID] = hdr.NumChunks // update cache
			}
			break
		case chunkMagic:
			streamID := binary.LittleEndian.Uint32(buf[4:])
			chunkID := binary.LittleEndian.Uint32(buf[8:])
			if tmp, err := mkStreamDir(tmpDir, streamID); err != nil {
				log.Println(err)
			} else {
				// Write Chunk File
				chunkFile := filepath.Join(tmp, strconv.FormatUint(uint64(chunkID), 16))
				err = ioutil.WriteFile(chunkFile, buf[12:], 0600)
				if err != nil {
					log.Println(err)
				}
				// Are we done?
				numChunks := chunksForStream(tmpDir, streamID)
				if numChunks > 0 {
					if isStreamComplete(tmpDir, streamID, numChunks) {
						completeFile(tmpDir, streamID, rxDir)
					}
				}
			}
			break
		default:
			log.Println("Magic Doesn't Match!")
			continue
		}
	}
}
