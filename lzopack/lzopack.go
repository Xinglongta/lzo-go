// Example program for lzo.go
// This program is based heavily off the 'lzopack' example from the LZO library
// License: GPLv3 or later
// Copyright (C) 2011 Damian Gryski <damian@gryski.com>

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/dgryski/go-lzo"
)

var magicHeader = [...]byte{0x00, 0xe9, 0x4c, 0x5a, 0x4f, 0xff, 0x1a}

func read8(r io.Reader) uint {
	var u uint8
	err := binary.Read(r, binary.BigEndian, &u)
	if err != nil {
		fatal("read failed: ", err)
	}
	return uint(u)
}

func read32(r io.Reader) uint {
	var u uint32
	err := binary.Read(r, binary.BigEndian, &u)
	if err != nil {
		fatal("read failed: ", err)
	}
	return uint(u)
}

func write8(w io.Writer, c byte) {
	err := binary.Write(w, binary.BigEndian, c)
	if err != nil {
		fatal("write failed: ", err)
	}
}

func write32(w io.Writer, ui uint32) {
	err := binary.Write(w, binary.BigEndian, ui)
	if err != nil {
		fatal("write failed: ", err)
	}
}

func fatal(a ...interface{}) {
	s := fmt.Sprint(a...)
	fmt.Fprintln(os.Stderr, "FATAL:", s)
	os.Exit(1)
}

func compress(in *os.File, out *os.File, level uint, blocksize uint) {

	out.Write(magicHeader[:])

	write32(out, 1)                // flags
	write8(out, 1)                 // method
	write8(out, uint8(level&0xff)) // level
	write32(out, uint32(blocksize))

	h := adler32.New()

	var algorithm lzo.LzoAlgorithm

	if level == 1 {
		algorithm = lzo.BestSpeed
	} else {
		algorithm = lzo.BestCompression
	}

	z, _ := lzo.NewCompressor(algorithm)

	inb := make([]byte, blocksize)

	for {
		nr, err := in.Read(inb[:])

		if nr == 0 && err == io.EOF {
			break
		}

		if err != nil {
			fatal("read failed: ", err)
		}

		// update checksum
		h.Write(inb[:nr])

		// try to compress
		o, err := z.Compress(inb[:nr])
		if err != nil {
			fatal("compression failed: ", err)
		}

		compressedSize := len(o)

		// we didn't compress it
		if compressedSize > nr {
			write32(out, uint32(nr))
			write32(out, uint32(nr))
			out.Write(inb[:nr])
		} else {
			write32(out, uint32(nr))
			write32(out, uint32(compressedSize))
			out.Write(o)
		}
	}

	// eof marker
	write32(out, 0)

	// is this right?

	hashb := h.Sum(nil)
	sum := (uint32(hashb[0]) << 24) | (uint32(hashb[1]) << 16) | (uint32(hashb[2]) << 8) | (uint32(hashb[3]) << 0)

	binary.Write(out, binary.BigEndian, sum)

	return
}

func decompress(in *os.File, out *os.File) {

	var magic [7]byte

	_, err := io.ReadFull(in, magic[:])
	if err != nil || !bytes.Equal(magic[:], magicHeader[:]) {
		fatal("header error -- this file was not compressed with lzopack")
	}

	flags := read32(in)
	method := read8(in)
	level := read8(in)
	blockSize := read32(in)

	if method != 1 {
		fatal("header error - unknown compression method: ", method, " (level: ", level, ")")
	}

	if blockSize < 1024 || blockSize > 8*1024*1024 {
		fatal("header error -- invalid block size: ", blockSize)
	}

	z, _ := lzo.NewCompressor(lzo.Lzo1x_1)
	h := adler32.New()
	inb := make([]byte, blockSize+blockSize/16+64+3)

	for {

		uncompressedBlocksize := read32(in)

		// end of compressed blocks?
		if uncompressedBlocksize == 0 {
			break
		}

		outb := make([]byte, uncompressedBlocksize)

		compressedBlocksize := read32(in)
		_, err := io.ReadFull(in, inb[:compressedBlocksize])
		if err != nil {
			fatal("unexpected end of file")
		}

		if compressedBlocksize == uncompressedBlocksize {
			// data was uncompressible -- nothing to decompress
			out.Write(inb[:compressedBlocksize])
			h.Write(inb[:compressedBlocksize])
			continue
		}

		sz, err := z.Decompress(inb[:compressedBlocksize], outb)

		if sz != uncompressedBlocksize || err != nil {
			fatal("compressed data violation")
		}

		out.Write(outb[:sz])
		h.Write(outb[:sz]) // update hash
	}

	if flags&1 == 1 {
		checksum := read32(in)

		hashb := h.Sum(nil)
		sum := (uint(hashb[0]) << 24) | (uint(hashb[1]) << 16) | (uint(hashb[2]) << 8) | (uint(hashb[3]) << 0)

		if checksum != sum {
			fatal("checksum error -- data corrupted")
		}
	}
}

func main() {

	//        var flag_block_size *int = flag.Int("block-size", 256*1024, "block size to use for compression")
	const blocksize = 256 * 1024

	var level uint

	var c_duration float64 = 0.0
	var d_duration float64 = 0.0

	for a := 1; a <= 50; a++ {

		infile := "/home/duan/Desktop/block/"
		compressedfile := "/home/duan/Desktop/block/compressed/LZO-GO/"
		decompressedfile := "/home/duan/Desktop/block/decompressed/LZO-GO/"

		name := strconv.Itoa(a)

		infile = infile + name
		compressedfile = compressedfile + name + ".lzogo"
		decompressedfile = decompressedfile + name

		inFile, err := os.Open(infile)
		if err != nil {
			fatal("input file: ", err)
		}

		c_File, err := os.Create(compressedfile)
		if err != nil {
			fatal("output file: ", err)
		}
		defer c_File.Close()

		//level  = 1 for "use fastest compression algorithm"
		//level  = 9 for "use best compression algorithm"
		level = 1

		start := time.Now()
		compress(inFile, c_File, level, blocksize)
		end := time.Now()
		var dur_time time.Duration = end.Sub(start)
		var elapsed_nano int64 = dur_time.Nanoseconds()
		c_duration += float64(elapsed_nano) / 1000000

		c_File.Close()
		c_File, err = os.Open(compressedfile)
		if err != nil {
			fatal("input file: ", err)
		}

		d_File, err := os.Create(decompressedfile)
		if err != nil {
			fatal("output file: ", err)
		}
		defer d_File.Close()

		start = time.Now()
		decompress(c_File, d_File)
		end = time.Now()
		dur_time = end.Sub(start)
		elapsed_nano = dur_time.Nanoseconds()
		d_duration += float64(elapsed_nano) / 1000000

	}

	fmt.Println("Average compressed time is ", c_duration/50, "ms.\nAverage decompressed time is ", d_duration/50, "ms\n")

}
