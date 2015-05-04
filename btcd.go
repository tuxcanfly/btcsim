/*
 * Copyright (c) 2014-2015 Conformal Systems LLC <info@conformal.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/btcsuite/btcd/wire"
	rpc "github.com/btcsuite/btcrpcclient"
)

// btcdArgs contains all the args and data required to launch a btcd
// instance and connect the rpc client to it
type btcdArgs struct {
	RPCUser    string
	RPCPass    string
	Listen     string
	RPCListen  string
	RPCConnect string
	DataDir    string
	LogDir     string
	Profile    string
	DebugLevel string
	Extra      []string

	prefix       string
	exe          string
	endpoint     string
	certificates []byte
}

// newBtcdArgs returns a btcdArgs with all default values
func newBtcdArgs(prefix string) (*btcdArgs, error) {
	a := &btcdArgs{
		Listen:    "127.0.0.1:18555",
		RPCListen: "127.0.0.1:18556",
		RPCUser:   "user",
		RPCPass:   "pass",

		prefix:   prefix,
		exe:      "btcd",
		endpoint: "ws",
	}
	return a, nil
}

// SetDefaults sets the default values of args
// it creates tmp data and log directories and must
// be cleaned up by calling Cleanup
func (a *btcdArgs) SetDefaults() error {
	datadir, err := ioutil.TempDir("", a.prefix+"-data")
	if err != nil {
		return err
	}
	a.DataDir = datadir
	logdir, err := ioutil.TempDir("", a.prefix+"-logs")
	if err != nil {
		return err
	}
	a.LogDir = logdir
	cert, err := ioutil.ReadFile(CertFile)
	if err != nil {
		return err
	}
	a.certificates = cert
	return nil
}

// String returns a printable name of this instance
func (a *btcdArgs) String() string {
	return a.prefix
}

// Arguments returns an array of arguments that be used to launch the
// btcd instance
func (a *btcdArgs) Arguments() []string {
	args := []string{}
	// --simnet
	args = append(args, fmt.Sprintf("--%s", strings.ToLower(wire.SimNet.String())))
	if a.RPCUser != "" {
		// --rpcuser
		args = append(args, fmt.Sprintf("--rpcuser=%s", a.RPCUser))
	}
	if a.RPCPass != "" {
		// --rpcpass
		args = append(args, fmt.Sprintf("--rpcpass=%s", a.RPCPass))
	}
	if a.Listen != "" {
		// --listen
		args = append(args, fmt.Sprintf("--listen=%s", a.Listen))
	}
	if a.RPCListen != "" {
		// --rpclisten
		args = append(args, fmt.Sprintf("--rpclisten=%s", a.RPCListen))
	}
	if a.RPCConnect != "" {
		// --rpcconnect
		args = append(args, fmt.Sprintf("--rpcconnect=%s", a.RPCConnect))
	}
	// --rpccert
	args = append(args, fmt.Sprintf("--rpccert=%s", CertFile))
	// --rpckey
	args = append(args, fmt.Sprintf("--rpckey=%s", KeyFile))
	if a.DataDir != "" {
		// --datadir
		args = append(args, fmt.Sprintf("--datadir=%s", a.DataDir))
	}
	if a.LogDir != "" {
		// --logdir
		args = append(args, fmt.Sprintf("--logdir=%s", a.LogDir))
	}
	if a.Profile != "" {
		// --profile
		args = append(args, fmt.Sprintf("--profile=%s", a.Profile))
	}
	if a.DebugLevel != "" {
		// --debuglevel
		args = append(args, fmt.Sprintf("--debuglevel=%s", a.DebugLevel))
	}
	args = append(args, a.Extra...)
	return args
}

// Command returns Cmd of the btcd instance
func (a *btcdArgs) Command() *exec.Cmd {
	return exec.Command(a.exe, a.Arguments()...)
}

// RPCConnConfig returns the rpc connection config that can be used
// to connect to the btcd instance that is launched on Start
func (a *btcdArgs) RPCConnConfig() rpc.ConnConfig {
	return rpc.ConnConfig{
		Host:                 a.RPCListen,
		Endpoint:             a.endpoint,
		User:                 a.RPCUser,
		Pass:                 a.RPCPass,
		Certificates:         a.certificates,
		DisableAutoReconnect: true,
	}
}

// Cleanup removes the tmp data and log directories
func (a *btcdArgs) Cleanup() error {
	dirs := []string{
		a.LogDir,
		a.DataDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return err
}
