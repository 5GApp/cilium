package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/noironetworks/cilium-net/bpf/lxcmap"
	"github.com/noironetworks/cilium-net/cilium-net-daemon/daemon"
	s "github.com/noironetworks/cilium-net/cilium-net-daemon/server"
	common "github.com/noironetworks/cilium-net/common"

	"github.com/noironetworks/cilium-net/Godeps/_workspace/src/github.com/op/go-logging"
	//"github.com/noironetworks/cilium-net/Godeps/_workspace/src/github.com/vishvananda/netlink"
)

const (
	logsDateFormat    = `-2006-01-02`
	logNameTimeFormat = time.RFC3339
)

var (
	socketPath   string
	logLevel     string
	nodeAddrStr  string
	NodeAddr     net.IP
	device       string
	libDir       string
	runDir       string
	lxcMap       *lxcmap.LxcMap
	log          = logging.MustGetLogger("cilium-net")
	stdoutFormat = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	fileFormat = logging.MustStringFormatter(
		`%{time:` + time.RFC3339Nano + `} ` + os.Getenv("HOSTNAME") + ` %{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}`,
	)
)

func setupLOG() {
	level, err := logging.LogLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}

	logTimename := time.Now().Format(logNameTimeFormat)
	ciliumLogsDir := os.TempDir() + string(os.PathSeparator) + "cilium-logs"
	if err := os.MkdirAll(ciliumLogsDir, 0755); err != nil {
		log.Error("Error while creating directory: %v", err)
	}

	fo, err := os.Create(ciliumLogsDir + string(os.PathSeparator) + "cilium-net-log-" + logTimename + ".log")
	if err != nil {
		log.Error("Error while creating log file: %v", err)
	}

	fileBackend := logging.NewLogBackend(fo, "", 0)

	fBF := logging.NewBackendFormatter(fileBackend, fileFormat)

	backend := logging.NewLogBackend(os.Stderr, "", 0)
	oBF := logging.NewBackendFormatter(backend, fileFormat)

	backendLeveled := logging.SetBackend(fBF, oBF)
	backendLeveled.SetLevel(level, "")
	log.SetBackend(backendLeveled)
}

func initBPF() {
	var args []string

	if device != "undefined" {
		args = []string{libDir, NodeAddr.String(), "direct", device}
	} else {
		args = []string{libDir, NodeAddr.String(), "vxlan"}
	}

	if err := os.Chdir(runDir); err != nil {
		log.Fatalf("Could not change to runtime directory %s: \"%s\"",
			runDir, err)
		return
	}

	f, err := os.Create("./globals/node_config.h")
	if err != nil {
		log.Warningf("Failed to create node configuration file: %s", err)
		fmt.Errorf("Failed to create node configuration file: \"%s\"", err)
		return

	}

	nodeMac, _ := net.ParseMAC("de:ad:be:ef:c0:de")

	fmt.Fprintf(f, ""+
		"/*\n"+
		" * Node MAC: %s\n"+
		" * Node IP: %s\n"+
		" */\n\n",
		nodeMac, NodeAddr.String())

	if logLevel == "debug" {
		f.WriteString("#define DEBUG\n")
	}

	fmt.Fprintf(f, "#define NODE_ID %#x\n", common.NodeAddr2ID(NodeAddr))
	f.WriteString(common.FmtDefineAddress("NODE_MAC", nodeMac))
	f.WriteString(common.FmtDefineArray("ROUTER_IP", NodeAddr))

	f.Close()

	out, err := exec.Command(libDir+"/init.sh", args...).CombinedOutput()
	if err != nil {
		log.Warningf("Command execution failed: %s", err)
		log.Warningf("Command output:\n%s", out)
		return
	}

	lxcMap, err = lxcmap.OpenMap(common.BPFMap)
	if err != nil {
		log.Warningf("Could not create BPF map '%s': %s", common.BPFMap, err)
		return
	}
}

func init() {
	flag.StringVar(&logLevel, "l", "info", "Set log level, valid options are (debug|info|warning|error|fatal|panic)")
	flag.StringVar(&socketPath, "s", common.CiliumSock, "Sets the socket path to listen for connections")
	flag.StringVar(&nodeAddrStr, "n", "", "IPv6 address of node, must be in correct format")
	flag.StringVar(&device, "d", "undefined", "Device to snoop on")
	flag.StringVar(&libDir, "D", "/usr/lib/cilium", "Cilium library directory")
	flag.StringVar(&runDir, "R", "/var/run/cilium", "Runtime data directory")
	flag.Parse()

	setupLOG()

	if nodeAddrStr == "" {
		var err error
		nodeAddrStr, err = common.GenerateV6Prefix()
		if err != nil {
			log.Fatalf("Unable to generate IPv6 prefix: %s\n", err)
		}

		log.Infof("Generated IPv6 prefix: %s\n", nodeAddrStr)
	}

	addr := net.ParseIP(nodeAddrStr)
	if addr == nil {
		log.Fatalf("Invalid node address \"%s\", please specifcy node address using -n", nodeAddrStr)
		return
	}

	if !common.ValidNodeAddress(addr) {
		log.Fatalf("Invalid node address: %s", nodeAddrStr)
	}

	var err error

	NodeAddr, _, err = net.ParseCIDR(addr.String() + "/64")
	if err != nil {
		log.Fatalf("Invalid CIDR %s", addr.String())
		return
	}

	initBPF()
}

func main() {
	d := daemon.NewDaemon(libDir, lxcMap)
	server, err := s.NewServer(socketPath, d)
	if err != nil {
		log.Fatalf("Error while creating daemon: %s", err)
	}
	defer server.Stop()
	server.Start()
}
