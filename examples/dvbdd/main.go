package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ziutek/dvb/linuxdvb/demux"
	"github.com/ziutek/dvb/linuxdvb/frontend"
	"github.com/ziutek/dvb/ts"

	"github.com/ziutek/dvb/examples/internal"
)

func usage() {
	fmt.Fprintf(
		os.Stderr,
		"Usage: %s [OPTION] PID [PID...]\nOptions:\n",
		filepath.Base(os.Args[0]),
	)
	flag.PrintDefaults()
	os.Exit(1)
}

var (
	fe     frontend.Device
	filter demux.StreamFilter
)

func main() {
	src := flag.String(
		"src", "rf",
		"source: rf, udp, mcast",
	)
	laddr := flag.String(
		"laddr", "0.0.0.0:1234",
		"listen IP address and port or multicast GROUP:PORT@INTERFACE",
	)
	fpath := flag.String(
		"front", "/dev/dvb/adapter0/frontend0",
		"path to the frontend device",
	)
	dmxpath := flag.String(
		"demux", "/dev/dvb/adapter0/demux0",
		"path to the demux device",
	)
	dvrpath := flag.String(
		"dvr", "",
		"path to the dvr device (defaul use demux to read packets)",
	)
	sys := flag.String(
		"sys", "t",
		"delivery system type: t, s, s2, ca, cb, cc",
	)
	channel := flag.String("channel", "", "channel name")
	conf := flag.String("conf", "", "configuration file")
	// freq := flag.Float64(
	// 	"freq", 0,
	// 	"frequency [Mhz]",
	// )
	// sr := flag.Uint(
	// 	"sr", 0,
	// 	"symbol rate [kBd]",
	// )
	pol := flag.String(
		"pol", "h",
		"polarization: h, v",
	)
	count := flag.Uint64(
		"count", 0,
		"number of MPEG-TS packets to process (default 0 means infinity)",
	)
	bw := flag.Uint(
		"bw", 0,
		"bandwidth [MHz] (default 0 means automatic)",
	)
	out := flag.String(
		"out", "",
		"output to the specified file or UDP address and port (default read and discard all packets)",
	)
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() == 0 {
		usage()
	}

	file, _ := os.Open(*conf)
    defer file.Close()

    scanner := bufio.NewScanner(file)
    inTargetSection := false
    var freq int64
    var sr uint
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())

        if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
            section := line[1 : len(line)-1]
            if section == *channel {
                inTargetSection = true
            } else {
                inTargetSection = false
            }
        }

        if inTargetSection {
            if strings.HasPrefix(line, "FREQUENCY") {
                parts := strings.Split(line, "=")
                if len(parts) == 2 {
                    frequencyStr := strings.TrimSpace(parts[1])
                    freq, _ = strconv.ParseInt(frequencyStr, 10, 64)
                    freq /= 1000000
                }
            }
            if strings.HasPrefix(line, "SYMBOL_RATE") {
                parts := strings.Split(line, "=")
                if len(parts) == 2 {
                    symbolRateStr := strings.TrimSpace(parts[1])
                    symbolRateInt, _ := strconv.ParseUint(symbolRateStr, 10, 32)
                    sr = uint(symbolRateInt / 1000)
                }
            }
        }
    }

	// fmt.Println("freq: ", freq)
	// fmt.Println("sr: ", sr)

	pids := make([]int16, flag.NArg())
	for i, a := range flag.Args() {
		pid, err := strconv.ParseInt(a, 0, 64)
		checkErr(err)
		if uint64(pid) > 8192 {
			die(a + " isn't in valid PID range [0, 8192]")
		}
		pids[i] = int16(pid)
	}

	var w ts.PktWriter
	switch {
	case *out == "":
		w = outputDiscard{}
	case strings.IndexByte(*out, ':') != -1:
		w = newOutputUDP("", *out)
	default:
		w = newOutputFile(*out)
	}

	var (
		r   ts.PktReader
		err error
	)
	switch *src {
	case "rf":
		fe, err = internal.Tune(*fpath, *sys, *pol, int64(freq*1e6), int(*bw*1e6), sr*1e3)
		checkErr(err)
		checkErr(internal.WaitForTune(fe, time.Now().Add(5*time.Second), true))
		r, filter = setFilter(*dmxpath, *dvrpath, pids)
	case "udp":
		r, err = internal.ListenUDP(*laddr, pids...)
		checkErr(err)
	case "mcast":
		r, err = internal.ListenMulticastUDP(*laddr, pids...)
		checkErr(err)
	default:
		die("Unknown source: " + *src)
	}

	pkt := new(ts.ArrayPkt)

	if *count == 0 {
		for {
			checkErr(r.ReadPkt(pkt))
			checkErr(w.WritePkt(pkt))
		}
	}
	for n := *count; n != 0; n-- {
		checkErr(r.ReadPkt(pkt))
		checkErr(w.WritePkt(pkt))
	}
}
