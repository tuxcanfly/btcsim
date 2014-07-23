package main

// place and start of file io utils for sim configurations

import (
	"encoding/csv"
	"io"
	"os"
	"strconv"
)

// reads csv of two columns of ints
func readCSV(f string) (map[int]int, error) {
	file, err := os.Open(f)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(file)
	reader.Comma = ','
	params := make(map[int]int)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		k, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, err
		}
		v, err := strconv.Atoi(row[1])
		if err != nil {
			return nil, err
		}
		params[k] = v
	}
	return params, nil
}
