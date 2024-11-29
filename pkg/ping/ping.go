package ping

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Pinger struct {
	host     string
	interval time.Duration
}

func NewPinger(host string, interval int) *Pinger {
	return &Pinger{
		host:     host,
		interval: time.Duration(interval) * time.Second,
	}
}

var PING_LINE = regexp.MustCompile(`^\d+ bytes from \d+.\d+.\d+.\d+: icmp_seq=\d+ ttl=\d+ time=(\d+.\d+) ms$`)

func processLine(line string) (time.Duration, error) {
	matches := PING_LINE.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, fmt.Errorf("failed to parse line: %s", line)
	}

	return time.ParseDuration(fmt.Sprintf("%sms", matches[1]))
}

func (p *Pinger) Run(ctx context.Context) (chan time.Duration, chan error) {
	pings := make(chan time.Duration)
	errs := make(chan error)

	go func() {
		defer close(pings)
		defer close(errs)

		cmd := exec.Command("ping", p.host, "-i", fmt.Sprintf("%d", p.interval/time.Second))
		stdout, err := cmd.StdoutPipe()

		if err != nil {
			errs <- err
			return
		}

		if err := cmd.Start(); err != nil {
			errs <- err
			return
		}

		go func() {
			scanner := bufio.NewScanner(stdout)

			for scanner.Scan() {
				line := scanner.Text()
				if len(line) < 1 {
					continue
				}

				if strings.HasPrefix(line, "PING") {
					continue
				}

				duration, err := processLine(line)
				if err != nil {
					// fmt.Printf("failed to process line: %s\n", err)
					continue
				}

				pings <- duration
			}
		}()

		finished := make(chan error)
		go func() {
			defer close(errs)

			if err := cmd.Wait(); err != nil {
				finished <- err
			}

			return
		}()

		select {
		case <-ctx.Done():
			errs <- nil
		case err := <-finished:
			errs <- err
		}
	}()

	return pings, errs
}
