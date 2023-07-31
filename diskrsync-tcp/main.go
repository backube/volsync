/*
Copyright © 2023 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/dop251/diskrsync"
	"github.com/dop251/spgz"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type options struct {
	noCompress bool
	verbose    bool
}

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [devicepath] [flags]\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	var (
		sourceMode    = flag.Bool("source", false, "Source mode")
		targetMode    = flag.Bool("target", false, "Target mode")
		targetAddress = flag.String("target-address", "", "address of the server, source only")
		controlFile   = flag.String("control-file", "", "name and path to file to write when finished")
		port          = flag.Int("port", 8000, "port to listen on or connect to")
	)
	opts := options{}

	flag.BoolVar(&opts.noCompress, "no-compress", false, "Store target as a raw file")
	flag.BoolVar(&opts.verbose, "verbose", true, "Print statistics, progress, and some debug info")

	zapopts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
		DestWriter:  os.Stdout,
	}
	zapopts.BindFlags(flag.CommandLine)

	// Import flags into pflag so they can be bound by viper
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()
	logger := zap.New(zap.UseFlagOptions(&zapopts))
	if *sourceMode && !*targetMode {
		if targetAddress == nil || *targetAddress == "" {
			fmt.Fprintf(os.Stderr, "target-address must be specified with source flag\n")
			usage()
			os.Exit(1)
		}
		if err := connectToTarget(os.Args[1], *targetAddress, *port, &opts, logger); err != nil {
			logger.Error(err, "Unable to connect to target", "source file", os.Args[1], "target address", *targetAddress)
			os.Exit(1)
		}
	} else if *targetMode && !*sourceMode {
		if controlFile == nil || *controlFile == "" {
			fmt.Fprintf(os.Stderr, "control-file must be specified with target flag\n")
			usage()
			os.Exit(1)
		}
		defer func() {
			logger.Info("Writing control file", "file", *controlFile)
			if err := createControlFile(*controlFile); err != nil {
				logger.Error(err, "Unable to create control file")
			}
		}()
		if err := startServer(os.Args[1], *port, &opts, logger); err != nil {
			logger.Error(err, "Unable to start server to write to file", "target file", os.Args[1])
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Either source or target must be defined\n")
		usage()
		os.Exit(1)
	}
	logger.Info("Successfully completed sync")
}

func createControlFile(fileName string) error {
	if err := os.MkdirAll(filepath.Dir(fileName), 0755); err != nil {
		return err
	}
	_, err := os.Create(fileName)
	return err
}

func connectToTarget(sourceFile, targetAddress string, port int, opts *options, logger logr.Logger) error {
	f, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	logger.Info("Opened filed", "file", sourceFile)
	defer f.Close()
	var src io.ReadSeeker

	// Try to open as an spgz file
	sf, err := spgz.NewFromFile(f, os.O_RDONLY)
	if err != nil {
		if err != spgz.ErrInvalidFormat {
			return err
		}
		logger.Info("Not an spgz file")
		src = f
	} else {
		logger.Info("spgz file")
		src = sf
	}

	size, err := src.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	_, err = src.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", targetAddress, port))
	if err != nil {
		return err
	}
	logger.Info("source", "size", size)
	calcProgress := &progress{
		progressType: "calc progress",
		logger:       logger,
	}
	syncProgress := &progress{
		progressType: "sync progress",
		logger:       logger,
	}
	err = diskrsync.Source(src, size, conn, conn, true, opts.verbose, calcProgress, syncProgress)
	cerr := conn.Close()
	if err == nil {
		err = cerr
	}
	return err
}

func startServer(targetFile string, port int, opts *options, logger logr.Logger) error {
	var w spgz.SparseFile
	useReadBuffer := false

	f, err := os.OpenFile(targetFile, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	logger.Info("Opened file", "file", targetFile)
	info, err := f.Stat()
	if err != nil {
		return err
	}
	logger.Info("file info", "info", info)

	if info.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0 {
		logger.Info("device file?")
		w = spgz.NewSparseFileWithoutHolePunching(f)
		useReadBuffer = true
	} else if !opts.noCompress {
		sf, err := spgz.NewFromFileSize(f, os.O_RDWR|os.O_CREATE, diskrsync.DefTargetBlockSize)
		if err != nil {
			if err != spgz.ErrInvalidFormat {
				if err == spgz.ErrPunchHoleNotSupported {
					err = fmt.Errorf("target does not support compression. Try with -no-compress option (error was '%v')", err)
				}
				return err
			}
		} else {
			w = &diskrsync.FixingSpgzFileWrapper{SpgzFile: sf}
		}
	}

	if w == nil {
		w = spgz.NewSparseFileWithFallback(f)
		useReadBuffer = true
	}

	defer func() {
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}()

	size, err := w.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	_, err = w.Seek(0, io.SeekStart)
	logger.Info("Size", "size", size)

	if err != nil {
		return err
	}

	logger.Info("Listening for tcp connection", "port", fmt.Sprintf(":%d", port))
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	conn, err := listener.Accept()
	if err != nil {
		return err
	}

	calcProgress := &progress{
		progressType: "calc progress",
		logger:       logger,
	}
	syncProgress := &progress{
		progressType: "sync progress",
		logger:       logger,
	}
	err = diskrsync.Target(w, size, conn, conn, useReadBuffer, opts.verbose, calcProgress, syncProgress)
	if err != nil {
		return err
	}
	return nil
}

type progress struct {
	total        int64
	current      int64
	progressType string
	lastUpdate   time.Time
	logger       logr.Logger
}

func (p *progress) Start(size int64) {
	p.total = size
	p.current = int64(0)
	p.lastUpdate = time.Now()
	p.logger.Info(fmt.Sprintf("%s total size %d", p.progressType, p.total))
}

func (p *progress) Update(pos int64) {
	p.current = pos
	if time.Since(p.lastUpdate).Seconds() > time.Second.Seconds() {
		p.logger.Info(fmt.Sprintf("%s %.2f%%", p.progressType, (float64(p.current) / float64(p.total) * 100)))
		p.lastUpdate = time.Now()
	}
}
