package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	rpc "github.com/conformal/btcrpcclient"
	"github.com/conformal/btcwire"
)

// bitcoindArgs contains all the args and data required to launch a bitcoind
// instance and connect the rpc client to it
type bitcoindArgs struct {
	RPCUser    string
	RPCPass    string
	Listen     string
	Port       string
	RPCListen  string
	RPCPort    string
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

// newBitcoindArgs returns a bitcoindArgs with all default values
func newBitcoindArgs(prefix string) (*bitcoindArgs, error) {
	a := &bitcoindArgs{
		Listen:    "127.0.0.1",
		Port:      "18555",
		RPCListen: "127.0.0.1",
		RPCPort:   "18556",
		RPCUser:   "user",
		RPCPass:   "pass",

		prefix:   prefix,
		exe:      "bitcoind",
		endpoint: "ws",
	}
	if err := a.SetDefaults(); err != nil {
		return nil, err
	}
	return a, nil
}

// SetDefaults sets the default values of args
// it creates tmp data and log directories and must
// be cleaned up by calling Cleanup
func (a *bitcoindArgs) SetDefaults() error {
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

	ConfigFile, err := os.Create(filepath.Join(a.DataDir,
		"bitcoin.conf"))
	if err != nil {
		return err
	}
	defer ConfigFile.Close()
	_, err = fmt.Fprintf(ConfigFile, "rpcuser=%s\n", a.RPCUser)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(ConfigFile, "rpcpassword=%s", a.RPCPass)
	if err != nil {
		return err
	}
	a.certificates = cert
	return nil
}

// String returns a printable name of this instance
func (a *bitcoindArgs) String() string {
	return a.prefix
}

// Arguments returns an array of arguments that be used to launch the
// bitcoind instance
func (a *bitcoindArgs) Arguments() []string {
	args := []string{}
	// --simnet
	args = append(args, fmt.Sprintf("--%s", strings.ToLower(btcwire.SimNet.String())))
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
	if a.Port != "" {
		// --port
		args = append(args, fmt.Sprintf("--port=%s", a.Port))
	}
	if a.RPCListen != "" {
		// --rpclisten
		args = append(args, fmt.Sprintf("--rpclisten=%s", a.RPCListen))
	}
	if a.RPCPort != "" {
		// --rpcport
		args = append(args, fmt.Sprintf("--rpcport=%s", a.RPCPort))
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

// Command returns Cmd of the bitcoind instance
func (a *bitcoindArgs) Command() *exec.Cmd {
	return exec.Command(a.exe, a.Arguments()...)
}

// RPCConnConfig returns the rpc connection config that can be used
// to connect to the bitcoind instance that is launched on Start
func (a *bitcoindArgs) RPCConnConfig() rpc.ConnConfig {
	host := fmt.Sprintf("%s:%s", a.RPCListen, a.RPCPort)
	return rpc.ConnConfig{
		Host:                 host,
		User:                 a.RPCUser,
		Pass:                 a.RPCPass,
		DisableAutoReconnect: true,
		HttpPostMode:         true,
	}
}

// Cleanup removes the tmp data and log directories
func (a *bitcoindArgs) Cleanup() error {
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
