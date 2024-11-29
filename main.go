package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"
	"ponglehub.co.uk/nettest/pkg/ping"
)

func main() {
	app := &cli.App{
		Name:  "network-test",
		Usage: "A simple network testing CLI",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "interval",
				Value:   1,
				Usage:   "Interval in seconds",
				Aliases: []string{"d"},
			},
			&cli.Int64Flag{
				Name:    "window",
				Value:   5,
				Usage:   "Window size for stats calculation",
				Aliases: []string{"w"},
			},
			&cli.StringFlag{
				Name:  "host",
				Value: "google.co.uk",
				Usage: "hostname to ping",
			},
		},
		Action: func(c *cli.Context) error {
			host := c.String("host")
			interval := c.Int("interval")
			window := c.Int64("window")

			return test(c.Context, host, interval, window)
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

type Window struct {
	Min   int64
	Max   int64
	Total int64
	Count int
}

func (w *Window) Update(duration int64) {
	if w.Min == 0 || duration < w.Min {
		w.Min = duration
	}

	if duration > w.Max {
		w.Max = duration
	}

	w.Total += duration
	w.Count++
}

func (w *Window) Reset() {
	w.Min = 0
	w.Max = 0
	w.Total = 0
	w.Count = 0
}

func (w *Window) Average() int {
	if w.Count == 0 {
		return 0
	}

	return int(w.Total) / w.Count
}

func (w *Window) String() string {
	return fmt.Sprintf("Min: %dms, Max: %dms, Avg: %dms", w.Min, w.Max, w.Average())
}

type Histogram struct {
	thresholds []int64
	buckets    []int
	total      int
}

func NewHistogram(thresholds []int64) Histogram {
	return Histogram{
		thresholds: thresholds,
		buckets:    make([]int, len(thresholds)),
	}
}

func (h *Histogram) Update(duration int64) {
	for i, threshold := range h.thresholds {
		if duration <= threshold {
			h.buckets[i]++
			break
		}
	}

	h.total++
}

type Stats struct {
	windowSize  time.Duration
	windowStart time.Time
	window      Window
	lastWindow  Window
	totals      Window
	histogram   Histogram
}

func (s *Stats) Update(duration int64) {
	s.window.Update(duration)
	s.totals.Update(duration)
	s.histogram.Update(duration)

	if time.Now().Sub(s.windowStart).Seconds() > s.windowSize.Seconds() {
		s.lastWindow = s.window
		s.window.Reset()
		s.windowStart = time.Now()
	}
}

func (s *Stats) String() string {
	return fmt.Sprintf("Window - %s\nTotals - %s", s.lastWindow.String(), s.totals.String())
}

func (s *Stats) PrintHistogram() string {
	var lines []string

	max := 0
	for _, count := range s.histogram.buckets {
		if count > max {
			max = count
		}
	}

	lines = append(lines, fmt.Sprintf("Histogram, Total: %d", s.histogram.total))

	for i, threshold := range s.histogram.thresholds {
		length := int(float64(s.histogram.buckets[i]) / float64(max) * 50)
		lines = append(lines, fmt.Sprintf("%5dms : %s", threshold, strings.Repeat("█", length)))
	}

	return strings.Join(lines, "\n")
}

type model struct {
	ctx      context.Context
	host     string
	interval int
	window   int64
	pings    chan time.Duration
	errs     chan error
	stats    Stats
}

type initParams struct {
	pings chan time.Duration
	errs  chan error
}

func (m model) tick() tea.Msg {
	select {
	case duration := <-m.pings:
		return duration
	case err := <-m.errs:
		return err
	case <-m.ctx.Done():
		return tea.Quit
	}
}

func (m model) Init() tea.Cmd {
	pings, err := ping.NewPinger(m.host, m.interval).Run(m.ctx)

	return func() tea.Msg {
		return initParams{
			pings: pings,
			errs:  err,
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case initParams:
		m.pings = msg.pings
		m.errs = msg.errs
		return m, m.tick
	case time.Duration:
		m.stats.Update(msg.Milliseconds())
		return m, m.tick
	case error:
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	return strings.Join([]string{
		"PING: " + m.host + " (interval: " + fmt.Sprintf("%d", m.interval) + "s)",
		"",
		m.stats.String(),
		"",
		m.stats.PrintHistogram(),
	}, "\n")
}

func test(ctx context.Context, host string, interval int, window int64) error {
	model := model{
		ctx:      ctx,
		host:     host,
		interval: interval,
		window:   window,
		stats: Stats{
			windowSize:  time.Duration(window) * time.Second,
			windowStart: time.Now(),
			window:      Window{},
			lastWindow:  Window{},
			totals:      Window{},
			histogram:   NewHistogram([]int64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000}),
		},
	}

	p := tea.NewProgram(model)
	_, err := p.Run()
	if err != nil {
		return err
	}

	return nil
}
