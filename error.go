package main

import "fmt"

// MissingCertPairFile is raised when one of the cert pair files is missing
type MissingCertPairFile string

func (m MissingCertPairFile) Error() string {
	return fmt.Sprintf("could not find TLS certificate pair file: %v", m)
}
