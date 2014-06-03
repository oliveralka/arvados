package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func ReadLineByLine(inp io.ReadCloser, out chan string, finish chan bool) {
	s := bufio.NewScanner(inp)
	for s.Scan() {
		out <- s.Text()
	}
	finish <- true
}

func OutputChannel(stdout chan string, stderr chan string) {
	for {
		select {
		case s, ok := <-stdout:
			if ok {
				fmt.Fprintln(os.Stdout, s)
			} else {
				return
			}
		case s, ok := <-stderr:
			if ok {
				fmt.Fprintln(os.Stderr, s)
			} else {
				return
			}
		}
	}
}

func PollCgroupStats(cgroup_path string, stderr chan string, poll int64) {
	//var last_usage int64 = 0
	var last_user int64 = 0
	var last_sys int64 = 0
	var last_cpucount int64 = 0

	type Disk struct {
		last_read  int64
		next_read  int64
		last_write int64
		next_write int64
	}

	disk := make(map[string]*Disk)

	//cpuacct_usage := fmt.Sprintf("%s/cpuacct.usage", cgroup_path)
	cpuacct_stat := fmt.Sprintf("%s/cpuacct.stat", cgroup_path)
	blkio_io_service_bytes := fmt.Sprintf("%s/blkio.io_service_bytes", cgroup_path)
	cpuset_cpus := fmt.Sprintf("%s/cpuset.cpus", cgroup_path)
	memory_stat := fmt.Sprintf("%s/memory.stat", cgroup_path)

	var ellapsed int64 = poll

	for {
		/*{
			c, _ := os.Open(cpuacct_usage)
			b, _ := ioutil.ReadAll(c)
			var next int64
			fmt.Sscanf(string(b), "%d", &next)
			if last_usage != 0 {
				stderr <- fmt.Sprintf("crunchstat: cpuacct.usage %v", (next-last_usage)/10000000)
			}
			//fmt.Printf("usage %d %d %d %d%%\n", last_usage, next, next-last_usage, (next-last_usage)/10000000)
			last_usage = next
			c.Close()
		}*/
		var cpus int64 = 0
		{
			c, _ := os.Open(cpuset_cpus)
			b, _ := ioutil.ReadAll(c)
			sp := strings.Split(string(b), ",")
			for _, v := range sp {
				var min, max int64
				n, _ := fmt.Sscanf(v, "%d-%d", &min, &max)
				if n == 2 {
					cpus += (max - min) + 1
				} else {
					cpus += 1
				}
			}

			if cpus != last_cpucount {
				stderr <- fmt.Sprintf("crunchstat: cpuset.cpus %v", cpus)
			}
			last_cpucount = cpus

			c.Close()
		}
		if cpus == 0 {
			cpus = 1
		}
		{
			c, _ := os.Open(cpuacct_stat)
			b, _ := ioutil.ReadAll(c)
			var next_user int64
			var next_sys int64
			fmt.Sscanf(string(b), "user %d\nsystem %d", &next_user, &next_sys)
			c.Close()

			if last_user != 0 {
				user_diff := next_user - last_user
				sys_diff := next_sys - last_sys
				// Assume we're reading stats based on 100
				// jiffies per second.  Because the ellaspsed
				// time is in milliseconds, we need to boost
				// that to 1000 jiffies per second, then boost
				// it by another 100x to get a percentage, then
				// finally divide by the actual ellapsed time
				// and the number of cpus to get average load
				// over the polling period.
				user_pct := (user_diff * 10 * 100) / (ellapsed * cpus)
				sys_pct := (sys_diff * 10 * 100) / (ellapsed * cpus)

				stderr <- fmt.Sprintf("crunchstat: cpuacct.stat user %v", user_pct)
				stderr <- fmt.Sprintf("crunchstat: cpuacct.stat sys %v", sys_pct)
			}

			/*fmt.Printf("user %d %d %d%%\n", last_user, next_user, next_user-last_user)
			fmt.Printf("sys %d %d %d%%\n", last_sys, next_sys, next_sys-last_sys)
			fmt.Printf("sum %d%%\n", (next_user-last_user)+(next_sys-last_sys))*/
			last_user = next_user
			last_sys = next_sys
		}
		{
			c, _ := os.Open(blkio_io_service_bytes)
			b := bufio.NewScanner(c)
			var device, op string
			var next int64
			for b.Scan() {
				if _, err := fmt.Sscanf(string(b.Text()), "%s %s %d", &device, &op, &next); err == nil {
					if disk[device] == nil {
						disk[device] = new(Disk)
					}
					if op == "Read" {
						disk[device].last_read = disk[device].next_read
						disk[device].next_read = next
						if disk[device].last_read > 0 {
							stderr <- fmt.Sprintf("crunchstat: blkio.io_service_bytes %s read %v", device, disk[device].next_read-disk[device].last_read)
						}
					}
					if op == "Write" {
						disk[device].last_write = disk[device].next_write
						disk[device].next_write = next
						if disk[device].last_write > 0 {
							stderr <- fmt.Sprintf("crunchstat: blkio.io_service_bytes %s write %v", device, disk[device].next_write-disk[device].last_write)
						}
					}
				}
			}
			c.Close()
		}

		{
			c, _ := os.Open(memory_stat)
			b := bufio.NewScanner(c)
			var stat string
			var val int64
			for b.Scan() {
				if _, err := fmt.Sscanf(string(b.Text()), "%s %d", &stat, &val); err == nil {
					if stat == "rss" {
						stderr <- fmt.Sprintf("crunchstat: memory.stat rss %v", val)
					}
				}
			}
			c.Close()
		}

		bedtime := time.Now()
		time.Sleep(time.Duration(poll) * time.Millisecond)
		morning := time.Now()
		ellapsed = morning.Sub(bedtime).Nanoseconds() / int64(time.Millisecond)
	}
}

func main() {

	var (
		cgroup_path    string
		cgroup_parent  string
		cgroup_cidfile string
		wait           int64
		poll           int64
	)

	flag.StringVar(&cgroup_path, "cgroup-path", "", "Direct path to cgroup")
	flag.StringVar(&cgroup_parent, "cgroup-parent", "", "Path to parent cgroup")
	flag.StringVar(&cgroup_cidfile, "cgroup-cid", "", "Path to container id file")
	flag.Int64Var(&wait, "wait", 5, "Maximum time (in seconds) to wait for cid file to show up")
	flag.Int64Var(&poll, "poll", 1000, "Polling frequency, in milliseconds")

	flag.Parse()

	logger := log.New(os.Stderr, "crunchstat: ", 0)

	if cgroup_path == "" && cgroup_cidfile == "" {
		logger.Fatal("Must provide either -cgroup-path or -cgroup-cid")
	}

	// Make output channel
	stdout_chan := make(chan string)
	stderr_chan := make(chan string)
	finish_chan := make(chan bool)
	defer close(stdout_chan)
	defer close(stderr_chan)
	defer close(finish_chan)

	go OutputChannel(stdout_chan, stderr_chan)

	var cmd *exec.Cmd

	if len(flag.Args()) > 0 {
		// Set up subprocess
		cmd = exec.Command(flag.Args()[0], flag.Args()[1:]...)

		logger.Print("Running ", flag.Args())

		// Forward SIGINT and SIGTERM to inner process
		term := make(chan os.Signal, 1)
		go func(sig <-chan os.Signal) {
			catch := <-sig
			if cmd.Process != nil {
				cmd.Process.Signal(catch)
			}
			logger.Print("caught signal:", catch)
		}(term)
		signal.Notify(term, syscall.SIGTERM)
		signal.Notify(term, syscall.SIGINT)

		// Funnel stdout and stderr from subprocess to output channels
		stdout_pipe, err := cmd.StdoutPipe()
		if err != nil {
			logger.Fatal(err)
		}
		go ReadLineByLine(stdout_pipe, stdout_chan, finish_chan)

		stderr_pipe, err := cmd.StderrPipe()
		if err != nil {
			logger.Fatal(err)
		}
		go ReadLineByLine(stderr_pipe, stderr_chan, finish_chan)

		// Run subprocess
		if err := cmd.Start(); err != nil {
			logger.Fatal(err)
		}
	}

	// Read the cid file
	if cgroup_cidfile != "" {
		// wait up to 'wait' seconds for the cid file to appear
		var i time.Duration
		for i = 0; i < time.Duration(wait)*time.Second; i += (100 * time.Millisecond) {
			f, err := os.Open(cgroup_cidfile)
			if err == nil {
				cid, err2 := ioutil.ReadAll(f)
				if err2 == nil && len(cid) > 0 {
					cgroup_path = string(cid)
					f.Close()
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		if cgroup_path == "" {
			logger.Printf("Could not read cid file %s", cgroup_cidfile)
		}
	}

	// add the parent prefix
	if cgroup_parent != "" {
		cgroup_path = fmt.Sprintf("%s/%s", cgroup_parent, cgroup_path)
	}

	logger.Print("Using cgroup ", cgroup_path)

	go PollCgroupStats(cgroup_path, stderr_chan, poll)

	// Wait for each of stdout and stderr to drain
	<-finish_chan
	<-finish_chan

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		} else {
			logger.Fatalf("cmd.Wait: %v", err)
		}
	}
}
