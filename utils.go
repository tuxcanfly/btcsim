package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"
)

// Row represents a row in the CSV file
// and holds the key and value ints
type Row struct {
	k int
	v int
}

// readCSV reads the given filename and
// returns a channel of rows
func readCSV(f string) chan *Row {
	rows := make(chan *Row)
	// Start a goroutine to read the CSV and
	// send the rows to the channel
	go func() {
		defer func() {
			close(rows)
		}()
		file, err := os.Open(f)
		defer file.Close()
		if err != nil {
			log.Fatalf("Cannot read CSV: %v", err)
			return
		}
		reader := csv.NewReader(file)
		reader.Comma = ','
		for {
			row, err := reader.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatalf("Error reading CSV: %v", err)
				return
			}
			k, err := strconv.Atoi(row[0])
			if err != nil {
				log.Fatalf("Error reading int key: %v", err)
				return
			}
			v, err := strconv.Atoi(row[1])
			if err != nil {
				log.Fatalf("Error reading int value: %v", err)
				return
			}
			rows <- &Row{k: k, v: v}
		}
	}()
	return rows
}
